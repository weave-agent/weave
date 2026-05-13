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
	bgMgr   *BackgroundManager
}

func init() {
	sdk.RegisterTool[BashConfig]("bash", func(cfg sdk.Config, bc BashConfig) (sdk.Tool, error) {
		timeout := time.Duration(bc.Timeout) * time.Second
		if timeout <= 0 {
			timeout = defaultTimeout
		}

		dir := dirFromConfig(cfg)

		return &tool{timeout: timeout, dir: dir, bgMgr: NewBackgroundManager()}, nil
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
		Description: "Execute a bash command and return its output. Supports background execution.",
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
				"run_in_background": map[string]any{
					"type":        "boolean",
					"description": "Run the command in the background and return a job ID immediately.",
				},
				"auto_background_after": map[string]any{
					"type":        "number",
					"description": "Start command synchronously and move to background after N seconds if still running. 0 disables auto-background.",
				},
			},
			"required": []string{ParamCommand},
		},
	}
}

func resolveTimeout(args map[string]any, base time.Duration) time.Duration {
	timeout := base
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	if v, ok := args["timeout"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			timeout = time.Duration(f) * time.Second
		}
	}

	return timeout
}

func (t *tool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	command, _ := args[ParamCommand].(string)
	if command == "" {
		return sdk.ToolResult{Content: "error: command is required", IsError: true}, nil
	}

	if s := sdk.GetSandboxer(); s != nil {
		wrapped, err := s.WrapCommand(command, t.dir)
		if err != nil {
			//nolint:nilerr // tool errors are returned via ToolResult.IsError
			return sdk.ToolResult{Content: "sandbox: " + err.Error(), IsError: true}, nil
		}

		command = wrapped
	}

	timeout := resolveTimeout(args, t.timeout)
	runInBackground, _ := args["run_in_background"].(bool)

	autoBackgroundAfter := 0

	if v, ok := args["auto_background_after"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			autoBackgroundAfter = int(f)
		}
	}

	bus := sdk.BusFromContext(ctx)

	if runInBackground {
		if t.bgMgr == nil {
			return sdk.ToolResult{Content: "error: background manager not available", IsError: true}, nil
		}

		job := t.bgMgr.Start(command, t.dir, timeout, bus)

		return sdk.ToolResult{
			Content: fmt.Sprintf("Background job started: %s\nCommand: %s\nWait for completion or check output later.", job.ID, command),
		}, nil
	}

	if autoBackgroundAfter > 0 {
		if t.bgMgr == nil {
			return sdk.ToolResult{Content: "error: background manager not available", IsError: true}, nil
		}

		job := t.bgMgr.Start(command, t.dir, timeout, bus)

		select {
		case <-job.done:
			return job.Result(), nil
		case <-time.After(time.Duration(autoBackgroundAfter) * time.Second):
			output := job.Output()
			result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
			formatted := formatResultWithTempFile(result, output)
			content := fmt.Sprintf("%s\n\nBackground job %s is still running.\nCommand: %s", formatted, job.ID, command)

			return sdk.ToolResult{Content: content}, nil
		}
	}

	return t.executeSync(ctx, command, timeout, bus)
}

func (t *tool) executeSync(ctx context.Context, command string, timeout time.Duration, bus sdk.Bus) (sdk.ToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	if t.dir != "" {
		if info, err := os.Stat(t.dir); err == nil && info.IsDir() {
			cmd.Dir = t.dir
		}
	}

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

	var outBuf strings.Builder

	outMu := &sync.Mutex{}

	var wg sync.WaitGroup

	wg.Add(2)
	go collectStream(stdoutPipe, "stdout", bus, command, &syncWriter{buf: &outBuf, mu: outMu}, &wg)
	go collectStream(stderrPipe, "stderr", bus, command, &syncWriter{buf: &outBuf, mu: outMu}, &wg)

	waitErr := cmd.Wait()
	wg.Wait()

	fullOutput := outBuf.String()

	if waitErr == nil {
		result := truncate.Truncate(fullOutput, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
		return sdk.ToolResult{Content: formatResultWithTempFile(result, fullOutput)}, nil
	}

	content, isErr := formatCmdError(fullOutput, waitErr, ctx)

	return sdk.ToolResult{Content: content, IsError: isErr}, nil
}

// syncWriter wraps a strings.Builder with a mutex for safe concurrent writes.
type syncWriter struct {
	buf *strings.Builder
	mu  *sync.Mutex
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, _ := w.buf.Write(p)

	return n, nil
}

// collectStream reads from r, writes raw bytes to outBuf, and publishes line
// events to bus when a complete line is read.
func collectStream(
	r io.Reader,
	stream string,
	bus sdk.Bus,
	command string,
	outBuf io.Writer,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	var lineBuf bytes.Buffer

	chunk := make([]byte, 4096)

	for {
		n, err := r.Read(chunk)
		if n > 0 {
			_, _ = outBuf.Write(chunk[:n])
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

func formatResultWithTempFile(result truncate.Result, fullOutput string) string {
	content := result.Format()
	if !result.Truncated {
		return content
	}

	tmpFile, err := os.CreateTemp("", "weave-bash-*.log")
	if err != nil {
		return content
	}

	_, writeErr := tmpFile.WriteString(fullOutput)

	closeErr := tmpFile.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpFile.Name())
		return content
	}

	return content + "\n\nFull output saved to: " + tmpFile.Name()
}

func formatCmdError(fullOutput string, err error, ctx context.Context) (string, bool) {
	result := truncate.Truncate(fullOutput, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	content := formatResultWithTempFile(result, fullOutput)

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() >= 0 {
		return fmt.Sprintf("%s\n[exit code %d]", content, exitErr.ExitCode()), false
	}

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return content + "\nerror: command timed out", true
	case ctx.Err() == context.Canceled:
		return content + "\nerror: command canceled", true
	default:
		return fmt.Sprintf("%s\nerror: %s", content, err), true
	}
}
