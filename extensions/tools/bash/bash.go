package bash

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"weave/internal/truncate"
	"weave/sdk"
)

const defaultTimeout = 120 * time.Second

type tool struct{}

func init() {
	sdk.RegisterTool("bash", func(_ sdk.Config) (sdk.Tool, error) {
		return &tool{}, nil
	})
}

func (t *tool) Name() string { return "bash" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "bash",
		Description: "Execute a bash command and return its output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute.",
				},
				"timeout": map[string]any{
					"type":        "number",
					"description": "Timeout in seconds. Defaults to 120.",
				},
			},
			"required": []string{"command"},
		},
	}
}

func (t *tool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return sdk.ToolResult{Content: "error: command is required", IsError: true}, nil
	}

	timeout := defaultTimeout

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
	out, err := cmd.CombinedOutput()

	result := truncate.Truncate(string(out), truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	if err == nil {
		return sdk.ToolResult{Content: result.Format()}, nil
	}

	content, isErr := formatCmdError(result, err, ctx)

	return sdk.ToolResult{Content: content, IsError: isErr}, nil
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
