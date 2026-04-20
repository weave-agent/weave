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
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	out, err := cmd.CombinedOutput()

	result := truncate.Truncate(string(out), truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	var content string
	var isErr bool

	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() >= 0 {
			content = fmt.Sprintf("%s\n[exit code %d]", result.Content, exitErr.ExitCode())
		} else if ctx.Err() == context.DeadlineExceeded {
			content = fmt.Sprintf("%s\nerror: command timed out", result.Content)
			isErr = true
		} else if ctx.Err() == context.Canceled {
			content = fmt.Sprintf("%s\nerror: command canceled", result.Content)
			isErr = true
		} else {
			content = fmt.Sprintf("%s\nerror: %s", result.Content, err)
			isErr = true
		}
	} else {
		content = result.Content
	}

	if result.Truncated {
		content = fmt.Sprintf("%s\n[output truncated: %d lines, %d bytes]", content, result.Lines, result.Bytes)
	}

	return sdk.ToolResult{Content: content, IsError: isErr}, nil
}
