package subagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// jsonEvent represents a JSON line emitted by the child subagent process.
type jsonEvent struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Model   string `json:"model"`
	Tool    string `json:"tool"`
	Args    any    `json:"args"`
	Output  string `json:"output"`
	Usage   *struct {
		Input  int `json:"input"`
		Output int `json:"output"`
	} `json:"usage,omitempty"`
}

// runSubagent executes a single subagent subprocess and returns its final output.
// The child process stdout is parsed as JSON lines; the content from the
// last "message_end" event is returned as the result.
// When subagentID is non-empty, it is passed to the child via --weave-subagent-id
// to enable inter-agent communication.
func runSubagent(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error) {
	if testRunSubagent != nil {
		return testRunSubagent(ctx, agent, prompt, cwd, subagentID)
	}

	cmd, cleanup, err := buildCommand(ctx, agent, prompt, cwd, subagentID)
	if err != nil {
		return "", err
	}
	defer cleanup()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("pipe stdout: %w", err)
	}

	// Redirect stderr to a pipe so it doesn't pollute the JSON stream on stdout.
	stderr, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		return "", fmt.Errorf("pipe stderr: %w", pipeErr)
	}

	go io.Copy(io.Discard, stderr) //nolint:errcheck // best-effort drain

	startErr := cmd.Start()
	if startErr != nil {
		return "", fmt.Errorf("start subagent: %w", startErr)
	}

	// Ensure the process and its process group are killed when the context is canceled.
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()

	result, err := parseJSONLines(stdout)
	if err != nil {
		_ = cmd.Wait()

		return "", err
	}

	waitErr := cmd.Wait()
	if waitErr != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("subagent aborted: %w", ctx.Err())
		}

		return "", fmt.Errorf("subagent exited with error: %w", waitErr)
	}

	return result, nil
}

// buildCommand constructs an exec.Cmd that runs the weave binary as a subagent.
// The prompt is written to a temporary file and passed via --weave-prompt-file.
// When subagentID is non-empty, --weave-subagent-id is included to enable
// inter-agent communication in the child process.
// A cleanup function is returned that removes the temporary prompt file.
func buildCommand(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (*exec.Cmd, func(), error) {
	f, err := os.CreateTemp("", "weave-subagent-prompt-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create prompt file: %w", err)
	}

	if _, writeErr := f.WriteString(prompt); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())

		return nil, nil, fmt.Errorf("write prompt file: %w", writeErr)
	}

	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(f.Name())

		return nil, nil, fmt.Errorf("close prompt file: %w", closeErr)
	}

	cleanup := func() { _ = os.Remove(f.Name()) }

	exe, err := os.Executable()
	if err != nil {
		cleanup()

		return nil, nil, fmt.Errorf("get executable: %w", err)
	}

	args := []string{
		"-p", "subagent", // dummy prompt to trigger headless mode
		"--weave-prompt-file=" + f.Name(),
	}

	// These flags require CLI support (Task 7). They are included now so the
	// command builder is complete; actual passthrough will work once the flags
	// are recognized by the weave CLI.
	args = append(args, "--output", "json")

	if len(agent.Tools) > 0 {
		args = append(args, "--tools", strings.Join(agent.Tools, ","))
	}

	if agent.Sandbox != "" {
		args = append(args, "--sandbox", agent.Sandbox)
	}

	if agent.Model != "" {
		args = append(args, "--model", agent.Model)
	}

	if subagentID != "" {
		args = append(args, "--weave-subagent-id", subagentID)
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if cwd != "" {
		cmd.Dir = cwd
	}

	return cmd, cleanup, nil
}

// parseJSONLines reads JSON lines from r and returns the content of the last
// "message_end" event. Non-JSON lines and unrecognized event types are ignored.
func parseJSONLines(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)

	var finalContent string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var evt jsonEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			// Non-JSON lines are ignored (could be log output or stderr redirects).
			continue
		}

		if evt.Type == "message_end" {
			finalContent = evt.Content
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read stdout: %w", err)
	}

	return finalContent, nil
}

// testRunSubagent is swapped out in tests to avoid spawning real subprocesses.
var testRunSubagent func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error)
