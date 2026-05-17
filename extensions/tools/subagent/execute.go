package subagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
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
// When subagentID is non-empty and broker is provided, the broker routes
// inter-agent messages and the child gets a stdin pipe for receiving them.
func runSubagent(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string, onEvent func(jsonEvent)) (string, error) {
	if testRunSubagent != nil {
		return testRunSubagent(ctx, agent, prompt, cwd, subagentID, broker, cfgPath, projectDir)
	}

	cmd, cleanup, err := buildCommand(ctx, agent, prompt, cwd, subagentID, cfgPath, projectDir)
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

	var stderrBuf strings.Builder

	var stderrWg sync.WaitGroup
	stderrWg.Go(func() {
		_, _ = io.Copy(&stderrBuf, stderr) // best-effort drain
	})

	var stdin io.WriteCloser
	if broker != nil && subagentID != "" {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return "", fmt.Errorf("pipe stdin: %w", err)
		}
	}

	startErr := cmd.Start()
	if startErr != nil {
		return "", fmt.Errorf("start subagent: %w", startErr)
	}

	if stdin != nil {
		broker.Register(subagentID, agent.Name, stdin)
	}

	var result string
	if broker != nil && subagentID != "" {
		result, err = broker.MonitorStdout(subagentID, stdout, onEvent)
	} else {
		result, err = parseJSONLines(stdout, onEvent)
	}

	if err != nil {
		_ = cmd.Cancel()
		// Drain stdout asynchronously so the child is not blocked on a full pipe.
		go func() { _, _ = io.Copy(io.Discard, stdout) }()

		_ = cmd.Wait()
		stderrWg.Wait()

		if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
			return "", fmt.Errorf("%w\nstderr: %s", err, stderrStr)
		}

		return "", err
	}

	waitErr := cmd.Wait()
	stderrWg.Wait()

	if waitErr != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("subagent aborted: %w", ctx.Err())
		}

		if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
			return "", fmt.Errorf("subagent exited with error: %w\nstderr: %s", waitErr, stderrStr)
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
func buildCommand(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID, cfgPath, projectDir string) (*exec.Cmd, func(), error) { //nolint:gocyclo // command construction with many conditional flags
	f, err := os.CreateTemp("", "weave-subagent-prompt-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create prompt file: %w", err)
	}

	// Restrict permissions on the prompt file since it may contain sensitive data.
	_ = os.Chmod(f.Name(), 0o600)

	// Combine agent system prompt with the user's task prompt.
	// Body is appended when both are present so that frontmatter system
	// overrides and markdown body instructions are both preserved.
	// If Body was already used as the System fallback in ParseAgent,
	// do not append it again.
	system := agent.System
	if agent.Body != "" && agent.System != agent.Body {
		if system != "" {
			system = system + "\n\n" + agent.Body
		} else {
			system = agent.Body
		}
	}

	fullPrompt := prompt
	if system != "" {
		fullPrompt = system + "\n\n" + prompt
	}

	if _, writeErr := f.WriteString(fullPrompt); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())

		return nil, nil, fmt.Errorf("write prompt file: %w", writeErr)
	}

	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(f.Name())

		return nil, nil, fmt.Errorf("close prompt file: %w", closeErr)
	}

	cleanup := func() { _ = os.Remove(f.Name()) }

	exe, err := subagentExecutable()
	if err != nil {
		cleanup()

		return nil, nil, fmt.Errorf("get executable: %w", err)
	}

	args := []string{
		"--weave-prompt-file=" + f.Name(),
		"--output=json",
	}

	tools := make([]string, 0, len(agent.Tools))
	tools = append(tools, agent.Tools...)

	if agent.Messaging {
		args = append(args, "--weave-messaging=true")

		seen := make(map[string]bool, len(tools))
		for _, t := range tools {
			seen[t] = true
		}

		for _, mt := range []string{"send_message", "broadcast_message", "list_agents"} {
			if !seen[mt] {
				tools = append(tools, mt)
			}
		}
	}

	if agent.Tools != nil {
		args = append(args, "--tools="+strings.Join(tools, ","))
	}

	if agent.Sandbox != "" {
		args = append(args, "--sandbox="+agent.Sandbox)
	}

	if agent.Model != "" {
		args = append(args, "--model="+agent.Model)
	}

	if subagentID != "" {
		args = append(args, "--subagent-id="+subagentID)
	}

	if cfgPath != "" {
		args = append(args, "--config="+cfgPath)
	}

	if projectDir != "" {
		args = append(args, "--weave-project-dir="+projectDir)
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	if cwd != "" {
		cmd.Dir = cwd
	}

	return cmd, cleanup, nil
}

func subagentExecutable() (string, error) {
	if launcherPath := os.Getenv("WEAVE_LAUNCHER_PATH"); launcherPath != "" {
		return launcherPath, nil
	}

	return "", errors.New("WEAVE_LAUNCHER_PATH is not set")
}

// parseJSONLines reads JSON lines from r and returns the content of the last
// "message_end" event. Non-JSON lines and unrecognized event types are ignored.
// When onEvent is non-nil, it is called for each successfully parsed JSON line.
func parseJSONLines(r io.Reader, onEvent func(jsonEvent)) (string, error) {
	scanner := bufio.NewScanner(r)

	// Increase buffer capacity to handle large JSON lines (e.g. message_end
	// events with full assistant content). Default 64 KiB is too small.
	const maxCapacity = 10 * 1024 * 1024 // 10 MB

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	var (
		finalContent  string
		sawMessageEnd bool
		jsonLines     int
	)

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

		jsonLines++

		if onEvent != nil {
			onEvent(evt)
		}

		if evt.Type == "message_end" {
			finalContent = evt.Content
			sawMessageEnd = true
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("subagent output line exceeded %d bytes: %w", maxCapacity, err)
	}

	if jsonLines == 0 {
		return "", errors.New("no valid JSON events from subagent")
	}

	if !sawMessageEnd {
		return "", errors.New("subagent stream ended without a message_end event")
	}

	return finalContent, nil
}

// testRunSubagent is swapped out in tests to avoid spawning real subprocesses.
var testRunSubagent func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error)
