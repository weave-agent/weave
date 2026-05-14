package subagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"weave/sdk"
)

// Bus topic constants for events published by the stdin listener.
const (
	topicSteer     = "agent.steer"
	topicInterrupt = "agent.interrupt"
)

// stdinListener reads JSON lines from stdin and dispatches inter-agent messages
// to the bus or to waiting tool calls.
type stdinListener struct {
	bus    sdk.Bus
	reader io.Reader

	mu     sync.Mutex
	respCh chan string // channel for list_agents_response delivery

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

var (
	// stdinListenerInst is the package-level stdin listener.
	stdinListenerInst *stdinListener
	stdinListenerMu   sync.Mutex
)

// startStdinListener starts the stdin listener if WEAVE_SUBAGENT_ID is set
// and it hasn't been started yet.
func startStdinListener(bus sdk.Bus) {
	if os.Getenv("WEAVE_SUBAGENT_ID") == "" {
		return
	}

	stdinListenerMu.Lock()
	defer stdinListenerMu.Unlock()

	if stdinListenerInst != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	sl := &stdinListener{
		bus:    bus,
		reader: stdinReader,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	stdinListenerInst = sl

	go sl.run()
}

// stopStdinListener stops the stdin listener. The context cancel unblocks
// run() immediately; the scanner goroutine will exit when the parent closes
// the child's stdin pipe after process shutdown.
func stopStdinListener() {
	stdinListenerMu.Lock()
	sl := stdinListenerInst
	stdinListenerInst = nil
	stdinListenerMu.Unlock()

	if sl != nil {
		sl.cancel()
		<-sl.done
	}
}

// run reads JSON lines from stdin and dispatches them.
func (sl *stdinListener) run() {
	defer close(sl.done)

	scanner := bufio.NewScanner(sl.reader)
	// Increase buffer capacity to handle large JSON lines (e.g. agent_msg
	// or inject events with large content). Default 64 KiB is too small.
	const maxCapacity = 10 * 1024 * 1024 // 10 MB

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)

		for scanner.Scan() {
			sl.handleLine(strings.TrimSpace(scanner.Text()))
		}
	}()

	select {
	case <-scanDone:
		if err := scanner.Err(); err != nil {
			sdk.Logger("subagent").Warn("scanner error", "error", err)
		}
	case <-sl.ctx.Done():
		// Give the scanner a brief moment to finish if stdin was just
		// closed. In tests the pipe closes before stopStdinListener,
		// so scanDone typically fires within microseconds.
		select {
		case <-scanDone:
			if err := scanner.Err(); err != nil {
				sdk.Logger("subagent").Warn("scanner error", "error", err)
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// handleLine dispatches a single JSON line.
func (sl *stdinListener) handleLine(line string) {
	if line == "" {
		return
	}

	var msg brokerMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return
	}

	switch msg.Type {
	case msgTypeInject:
		sl.bus.Publish(sdk.NewEvent(topicSteer, msg.Content))
	case msgTypeAgentMsg:
		content := fmt.Sprintf("[from %s] %s", msg.From, msg.Content)
		sl.bus.Publish(sdk.NewEvent(topicSteer, content))
	case msgTypeCancel:
		sl.bus.Publish(sdk.NewEvent(topicInterrupt, nil))
	case msgTypeListAgentsResp:
		sl.mu.Lock()
		ch := sl.respCh
		sl.mu.Unlock()

		if ch != nil {
			select {
			case ch <- formatRoster(msg.Agents):
			default:
			}
		}
	}
}

// setResponseChannel sets the channel for list_agents_response delivery.
func (sl *stdinListener) setResponseChannel(ch chan string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	sl.respCh = ch
}

// getStdinListener returns the global stdin listener, or nil if not started.
func getStdinListener() *stdinListener {
	stdinListenerMu.Lock()
	defer stdinListenerMu.Unlock()

	return stdinListenerInst
}
