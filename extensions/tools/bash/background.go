package bash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"weave/sdk"
	"weave/utils/truncate"
)

// BackgroundStartPayload is the payload for tool.bash.background_start events.
type BackgroundStartPayload struct {
	ID      string `json:"id"`
	Command string `json:"command"`
}

// BackgroundDonePayload is the payload for tool.bash.background_done events.
type BackgroundDonePayload struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// BackgroundJob represents a running background bash command.
type BackgroundJob struct {
	ID        string
	Command   string
	StartTime time.Time

	mu       sync.RWMutex
	output   strings.Builder
	exitErr  error
	exitCode int
	done     chan struct{}
	cancel   context.CancelFunc
}

// IsDone returns true if the background job has completed.
func (j *BackgroundJob) IsDone() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

// Output returns the current accumulated output of the job.
func (j *BackgroundJob) Output() string {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return j.output.String()
}

// ExitCode returns the exit code of the job. Only valid after IsDone() is true.
func (j *BackgroundJob) ExitCode() int {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return j.exitCode
}

// ExitError returns the exit error of the job. Only valid after IsDone() is true.
func (j *BackgroundJob) ExitError() error {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return j.exitErr
}

// Wait blocks until the background job completes.
func (j *BackgroundJob) Wait() {
	<-j.done
}

// Result returns a formatted ToolResult for the job's current or final output.
func (j *BackgroundJob) Result() sdk.ToolResult {
	output := j.Output()
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	content := formatResultWithTempFile(result, output)

	j.mu.RLock()
	exitCode := j.exitCode
	exitErr := j.exitErr
	j.mu.RUnlock()

	if exitErr == nil && exitCode == 0 {
		return sdk.ToolResult{Content: content}
	}

	if exitCode > 0 {
		return sdk.ToolResult{
			Content: fmt.Sprintf("%s\n[exit code %d]", content, exitCode),
			IsError: false,
		}
	}

	return sdk.ToolResult{
		Content: fmt.Sprintf("%s\nerror: %s", content, exitErr),
		IsError: true,
	}
}

func (j *BackgroundJob) run(command, dir string, timeout time.Duration, bus sdk.Bus) {
	defer close(j.done)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	j.mu.Lock()
	j.cancel = cancel
	j.mu.Unlock()

	defer cancel()

	var (
		exitCode  int
		exitError error
	)

	defer func() {
		j.mu.Lock()
		j.exitCode = exitCode
		j.exitErr = exitError
		j.mu.Unlock()

		if bus != nil {
			payload := BackgroundDonePayload{
				ID:       j.ID,
				Command:  j.Command,
				ExitCode: exitCode,
			}
			if exitError != nil {
				payload.Error = exitError.Error()
			}

			bus.Publish(sdk.NewEvent("tool.bash.background_done", payload))
		}
	}()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}

		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}

			return fmt.Errorf("bash: kill process: %w", err)
		}

		return nil
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		exitError = err
		return
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		exitError = err
		return
	}

	if err := cmd.Start(); err != nil {
		exitError = err
		return
	}

	if bus != nil {
		bus.Publish(sdk.NewEvent("tool.bash.background_start", BackgroundStartPayload{
			ID:      j.ID,
			Command: j.Command,
		}))
	}

	var wg sync.WaitGroup

	wg.Add(2)
	go j.collectStream(stdoutPipe, "stdout", bus, &wg)
	go j.collectStream(stderrPipe, "stderr", bus, &wg)

	waitErr := cmd.Wait()
	wg.Wait()

	if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok && exitErr.ExitCode() >= 0 {
		exitCode = exitErr.ExitCode()
	} else if waitErr != nil {
		exitError = waitErr
	}
}

func (j *BackgroundJob) collectStream(r io.Reader, stream string, bus sdk.Bus, wg *sync.WaitGroup) {
	defer wg.Done()

	var lineBuf bytes.Buffer

	chunk := make([]byte, 4096)

	for {
		n, err := r.Read(chunk)
		if n > 0 {
			j.mu.Lock()
			j.output.Write(chunk[:n])
			j.mu.Unlock()

			lineBuf.Write(chunk[:n])

			for {
				data := lineBuf.Bytes()

				before, after, found := bytes.Cut(data, []byte{'\n'})
				if !found {
					break
				}

				if bus != nil {
					bus.Publish(sdk.NewEvent("tool.bash.output", BashOutputPayload{
						Command: j.Command,
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
				Command: j.Command,
				Line:    lineBuf.String(),
				Stream:  stream,
			}))
		}
	}
}

// BackgroundManager manages a collection of background bash jobs.
type BackgroundManager struct {
	mu      sync.RWMutex
	jobs    map[string]*BackgroundJob
	counter int
}

// NewBackgroundManager creates a new background manager.
func NewBackgroundManager() *BackgroundManager {
	return &BackgroundManager{
		jobs: make(map[string]*BackgroundJob),
	}
}

// Start begins a new background job with the given command.
func (bm *BackgroundManager) Start(command, dir string, timeout time.Duration, bus sdk.Bus) *BackgroundJob {
	bm.mu.Lock()
	bm.counter++
	id := fmt.Sprintf("job-%d", bm.counter)
	job := &BackgroundJob{
		ID:        id,
		Command:   command,
		StartTime: time.Now(),
		done:      make(chan struct{}),
		cancel:    func() {}, // no-op until run() sets the real cancel
	}
	bm.jobs[id] = job
	bm.mu.Unlock()

	go job.run(command, dir, timeout, bus)

	return job
}

// Get retrieves a background job by ID.
func (bm *BackgroundManager) Get(id string) (*BackgroundJob, bool) {
	bm.mu.RLock()
	job, ok := bm.jobs[id]
	bm.mu.RUnlock()

	return job, ok
}

// Output returns the current output of the job with the given ID.
func (bm *BackgroundManager) Output(id string) (string, bool) {
	job, ok := bm.Get(id)
	if !ok {
		return "", false
	}

	return job.Output(), true
}

// Remove removes a background job from the manager.
func (bm *BackgroundManager) Remove(id string) {
	bm.mu.Lock()
	delete(bm.jobs, id)
	bm.mu.Unlock()
}

// Kill terminates the background job with the given ID.
func (bm *BackgroundManager) Kill(id string) error {
	job, ok := bm.Get(id)
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}

	job.mu.RLock()
	cancel := job.cancel
	job.mu.RUnlock()

	if cancel != nil {
		cancel()
	}

	return nil
}

// List returns all background jobs.
func (bm *BackgroundManager) List() []*BackgroundJob {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make([]*BackgroundJob, 0, len(bm.jobs))
	for _, job := range bm.jobs {
		result = append(result, job)
	}

	return result
}
