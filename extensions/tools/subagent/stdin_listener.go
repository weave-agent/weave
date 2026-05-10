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

	_, cancel := context.WithCancel(context.Background())
	sl := &stdinListener{
		bus:    bus,
		reader: stdinReader,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	stdinListenerInst = sl

	go sl.run()
}

// stopStdinListener stops the stdin listener and waits for it to finish.
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

	for scanner.Scan() {
		sl.handleLine(strings.TrimSpace(scanner.Text()))
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
