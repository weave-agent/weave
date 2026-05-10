package subagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"weave/sdk"
)

// backgroundAgent tracks the state of a running background subagent.
type backgroundAgent struct {
	ID       string
	Agent    *AgentDef
	Prompt   string
	CWD      string
	Status   string // "running", "completed", "failed"
	Result   string
	Err      error
	done     chan struct{}
	started  time.Time
	finished time.Time
}

// backgroundManager tracks all running background agents.
type backgroundManager struct {
	mu     sync.RWMutex
	agents map[string]*backgroundAgent
	bus    sdk.Bus
	broker *Broker
	ctx    context.Context
	cancel context.CancelFunc
}

func newBackgroundManager(broker *Broker) *backgroundManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &backgroundManager{
		agents: make(map[string]*backgroundAgent),
		broker: broker,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (bm *backgroundManager) setBus(bus sdk.Bus) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.bus = bus
}

func (bm *backgroundManager) spawn(agent *AgentDef, prompt, cwd, subagentID string) string {
	if subagentID == "" {
		subagentID = generateAgentID(agent.Name)
	}

	ba := &backgroundAgent{
		ID:      subagentID,
		Agent:   agent,
		Prompt:  prompt,
		CWD:     cwd,
		Status:  statusRunning,
		done:    make(chan struct{}),
		started: time.Now(),
	}

	bm.mu.Lock()
	bm.agents[subagentID] = ba
	bm.mu.Unlock()

	go func() {
		defer close(ba.done)

		// Use the manager's context so background agents are canceled
		// when the extension is closed.
		output, err := runSubagent(bm.ctx, agent, prompt, cwd, subagentID, bm.broker)

		bm.mu.Lock()
		ba.finished = time.Now()

		if err != nil {
			ba.Status = "failed"
			ba.Err = err
			ba.Result = err.Error()
		} else {
			ba.Status = "completed"
			ba.Result = output
		}
		bm.mu.Unlock()

		bm.notifyDone(ba)
	}()

	return subagentID
}

func (bm *backgroundManager) notifyDone(ba *backgroundAgent) {
	bm.mu.RLock()
	bus := bm.bus
	bm.mu.RUnlock()

	if bus == nil {
		return
	}

	payload := map[string]string{
		propID:     ba.ID,
		"status":   ba.Status,
		keyContent: ba.Result,
	}

	bus.Publish(sdk.NewEvent("subagent.done", payload))
}

func (bm *backgroundManager) get(id string) (*backgroundAgent, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	ba, ok := bm.agents[id]

	return ba, ok
}

func (bm *backgroundManager) await(ctx context.Context, id string) (*backgroundAgent, bool) {
	ba, ok := bm.get(id)
	if !ok {
		return nil, false
	}

	select {
	case <-ba.done:
		return ba, true
	case <-ctx.Done():
		return nil, false
	}
}

var agentIDCounter atomic.Uint64

func generateAgentID(name string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp + counter on entropy failure.
		return fmt.Sprintf("subagent_%s_%d_%d", name, time.Now().UnixNano(), agentIDCounter.Add(1))
	}

	return fmt.Sprintf("subagent_%s_%s", name, hex.EncodeToString(b))
}

// checkAgentTool implements sdk.Tool for checking background agent status.
type checkAgentTool struct {
	mgr *backgroundManager
}

func (t *checkAgentTool) Name() string { return "check_agent" }

func (t *checkAgentTool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "check_agent",
		Description: "Check the status and result of a background agent",
		Parameters: map[string]any{
			jsonType: "object",
			"properties": map[string]any{
				propID: map[string]any{jsonType: jsonString, propDescription: "Agent ID returned from background spawn"},
			},
			"required": []string{propID},
		},
	}
}

func (t *checkAgentTool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	idVal, ok := args[propID]
	if !ok {
		return sdk.ToolResult{Content: "missing required parameter: id", IsError: true}, nil
	}

	id, ok := idVal.(string)
	if !ok || id == "" {
		return sdk.ToolResult{Content: "id must be a non-empty string", IsError: true}, nil
	}

	ba, ok := t.mgr.get(id)
	if !ok {
		return sdk.ToolResult{Content: fmt.Sprintf("agent %q not found", id), IsError: true}, nil
	}

	result := map[string]any{
		propID:   ba.ID,
		"status": ba.Status,
	}

	if ba.Status != statusRunning {
		result["content"] = ba.Result
		if ba.Err != nil {
			result["error"] = ba.Err.Error()
		}
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol: errors in Content, not return
	}

	return sdk.ToolResult{Content: string(jsonBytes)}, nil
}

// awaitAgentTool implements sdk.Tool for awaiting background agent completion.
type awaitAgentTool struct {
	mgr *backgroundManager
}

func (t *awaitAgentTool) Name() string { return "await_agent" }

func (t *awaitAgentTool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "await_agent",
		Description: "Block until a background agent completes and return its result",
		Parameters: map[string]any{
			jsonType: "object",
			"properties": map[string]any{
				propID: map[string]any{jsonType: jsonString, propDescription: "Agent ID returned from background spawn"},
			},
			"required": []string{propID},
		},
	}
}

func (t *awaitAgentTool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	idVal, ok := args[propID]
	if !ok {
		return sdk.ToolResult{Content: "missing required parameter: id", IsError: true}, nil
	}

	id, ok := idVal.(string)
	if !ok || id == "" {
		return sdk.ToolResult{Content: "id must be a non-empty string", IsError: true}, nil
	}

	ba, ok := t.mgr.await(ctx, id)
	if !ok {
		return sdk.ToolResult{Content: fmt.Sprintf("agent %q not found", id), IsError: true}, nil
	}

	result := map[string]any{
		propID:     ba.ID,
		"status":   ba.Status,
		keyContent: ba.Result,
	}

	if ba.Err != nil {
		result["error"] = ba.Err.Error()
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol: errors in Content, not return
	}

	return sdk.ToolResult{Content: string(jsonBytes)}, nil
}
