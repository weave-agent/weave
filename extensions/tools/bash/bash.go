package bash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"weave/sdk"
	"weave/utils/truncate"
)

const defaultTimeout = 120 * time.Second

// ParamCommand is the tool parameter name for the command to execute.
const ParamCommand = "command"

// BashOutputPayload is the payload for tool.bash.output bus events.
type BashOutputPayload struct {
	Command string `json:"command"`
	Line    string `json:"line"`
	Stream  string `json:"stream"` // "stdout" or "stderr"
}

// BashConfig holds per-tool settings for the bash tool.
type BashConfig struct {
	Timeout int `json:"timeout" default:"120" env:"TIMEOUT"`
}

type tool struct {
	timeout time.Duration
	dir     string
}

func init() {
	sdk.RegisterTool[BashConfig]("bash", func(cfg sdk.Config, bc BashConfig) (sdk.Tool, error) {
		timeout := time.Duration(bc.Timeout) * time.Second
		if timeout <= 0 {
			timeout = defaultTimeout
		}

		dir := dirFromConfig(cfg)

		return &tool{timeout: timeout, dir: dir}, nil
	})
}

func dirFromConfig(cfg sdk.Config) string {
	if pd := cfg.ProjectDir(); pd != "" {
		return pd
	}

	if fp := cfg.FilePath(); fp != "" {
		dir := filepath.Dir(fp)
		// If config is inside .weave/ directory, go up one level to project root.
		if filepath.Base(dir) == ".weave" {
			return filepath.Dir(dir)
		}

		return dir
	}

	dir, _ := os.Getwd()

	return dir
}

func (t *tool) Name() string { return "bash" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "bash",
		Description: "Execute a bash command and return its output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				ParamCommand: map[string]any{
					"type":        "string",
					"description": "The bash command to execute.",
				},
				"timeout": map[string]any{
					"type":        "number",
					"description": "Timeout in seconds. Defaults to 120.",
				},
			},
			"required": []string{ParamCommand},
		},
	}
}

func (t *tool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	command, _ := args[ParamCommand].(string)
	if command == "" {
		return sdk.ToolResult{Content: "error: command is required", IsError: true}, nil
	}

	if s := sdk.GetSandboxer(); s != nil {
		wrapped, err := s.WrapCommand(command, t.dir)
		if err != nil {
			return sdk.ToolResult{Content: "sandbox: " + err.Error(), IsError: true}, nil
		}

		command = wrapped
	}

	timeout := t.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	if v, ok := args["timeout"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			timeout = time.Duration(f) * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}

		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}

		return fmt.Errorf("bash: kill process: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return sdk.ToolResult{Content: "error: " + err.Error(), IsError: true}, nil
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return sdk.ToolResult{Content: "error: " + err.Error(), IsError: true}, nil
	}

	if err := cmd.Start(); err != nil {
		return sdk.ToolResult{Content: "error: " + err.Error(), IsError: true}, nil
	}

	bus := sdk.BusFromContext(ctx)

	var outBuf strings.Builder

	var wg sync.WaitGroup

	wg.Add(2)
	go collectStream(stdoutPipe, "stdout", bus, command, &outBuf, &wg)
	go collectStream(stderrPipe, "stderr", bus, command, &outBuf, &wg)

	waitErr := cmd.Wait()
	wg.Wait()

	result := truncate.Truncate(outBuf.String(), truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	if waitErr == nil {
		return sdk.ToolResult{Content: result.Format()}, nil
	}

	content, isErr := formatCmdError(result, waitErr, ctx)

	return sdk.ToolResult{Content: content, IsError: isErr}, nil
}

// collectStream reads from r, writes raw bytes to outBuf, and publishes line
// events to bus when a complete line is read.
func collectStream(
	r io.Reader,
	stream string,
	bus sdk.Bus,
	command string,
	outBuf *strings.Builder,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	var lineBuf bytes.Buffer

	chunk := make([]byte, 4096)

	for {
		n, err := r.Read(chunk)
		if n > 0 {
			outBuf.Write(chunk[:n])
			lineBuf.Write(chunk[:n])

			for {
				data := lineBuf.Bytes()

				before, after, found := bytes.Cut(data, []byte{'\n'})
				if !found {
					break
				}

				if bus != nil {
					bus.Publish(sdk.NewEvent("tool.bash.output", BashOutputPayload{
						Command: command,
						Line:    string(before),
						Stream:  stream,
					}))
				}

				lineBuf.Reset()
				lineBuf.Write(after)
			}
		}

		if err != nil {
			break
		}
	}

	if lineBuf.Len() > 0 {
		if bus != nil {
			bus.Publish(sdk.NewEvent("tool.bash.output", BashOutputPayload{
				Command: command,
				Line:    lineBuf.String(),
				Stream:  stream,
			}))
		}
	}
}

func formatCmdError(result truncate.Result, err error, ctx context.Context) (string, bool) {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() >= 0 {
		return fmt.Sprintf("%s\n[exit code %d]", result.Format(), exitErr.ExitCode()), false
	}

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return result.Format() + "\nerror: command timed out", true
	case ctx.Err() == context.Canceled:
		return result.Format() + "\nerror: command canceled", true
	default:
		return fmt.Sprintf("%s\nerror: %s", result.Format(), err), true
	}
}
