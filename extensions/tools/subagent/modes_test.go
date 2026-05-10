package subagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunParallel_Success(t *testing.T) {
	original := testRunSubagent
	defer func() { testRunSubagent = original }()

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error) {
		return "result for: " + prompt, nil
	}

	agent := &AgentDef{Name: "test"}
	tasks := []any{
		map[string]any{"prompt": "task 1"},
		map[string]any{"prompt": "task 2"},
		map[string]any{"prompt": "task 3"},
	}

	result, err := runParallel(context.Background(), agent, tasks, "")
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "Task 1:")
	assert.Contains(t, result.Content, "result for: task 1")
	assert.Contains(t, result.Content, "Task 2:")
	assert.Contains(t, result.Content, "result for: task 2")
	assert.Contains(t, result.Content, "Task 3:")
	assert.Contains(t, result.Content, "result for: task 3")
}

func TestRunParallel_PartialFailure(t *testing.T) {
	original := testRunSubagent
	defer func() { testRunSubagent = original }()

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error) {
		if prompt == "fail" {
			return "", errors.New("task failed")
		}

		return "ok: " + prompt, nil
	}

	agent := &AgentDef{Name: "test"}
	tasks := []any{
		map[string]any{"prompt": "ok"},
		map[string]any{"prompt": "fail"},
		map[string]any{"prompt": "ok2"},
	}

	result, err := runParallel(context.Background(), agent, tasks, "")
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "ok: ok")
	assert.Contains(t, result.Content, "ERROR: task failed")
	assert.Contains(t, result.Content, "ok: ok2")
}

func TestRunParallel_AllFailure(t *testing.T) {
	original := testRunSubagent
	defer func() { testRunSubagent = original }()

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error) {
		return "", errors.New("all failed")
	}

	agent := &AgentDef{Name: "test"}
	tasks := []any{
		map[string]any{"prompt": "task 1"},
		map[string]any{"prompt": "task 2"},
	}

	result, err := runParallel(context.Background(), agent, tasks, "")
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "ERROR: all failed")
}

func TestRunParallel_Concurrency(t *testing.T) {
	original := testRunSubagent
	defer func() { testRunSubagent = original }()

	var mu sync.Mutex

	maxConcurrent := 0
	currentConcurrent := 0

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error) {
		mu.Lock()

		currentConcurrent++
		if currentConcurrent > maxConcurrent {
			maxConcurrent = currentConcurrent
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		currentConcurrent--
		mu.Unlock()

		return "done", nil
	}

	agent := &AgentDef{Name: "test"}
	tasks := []any{
		map[string]any{"prompt": "task 1"},
		map[string]any{"prompt": "task 2"},
		map[string]any{"prompt": "task 3"},
	}

	_, err := runParallel(context.Background(), agent, tasks, "")
	require.NoError(t, err)

	assert.GreaterOrEqual(t, maxConcurrent, 2, "expected at least 2 concurrent executions")
}

func TestRunParallel_EmptyTasks(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	result, err := runParallel(context.Background(), agent, []any{}, "")
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Empty(t, result.Content)
}

func TestRunParallel_InvalidTaskType(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tasks := []any{
		"not an object",
	}

	result, err := runParallel(context.Background(), agent, tasks, "")
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "expected object")
}

func TestRunParallel_MissingPromptField(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tasks := []any{
		map[string]any{"other": "value"},
	}

	result, err := runParallel(context.Background(), agent, tasks, "")
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing \"prompt\" field")
}

func TestRunChain_Success(t *testing.T) {
	original := testRunSubagent
	defer func() { testRunSubagent = original }()

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error) {
		return "processed: " + prompt, nil
	}

	agent := &AgentDef{Name: "test"}
	chain := []any{
		map[string]any{"prompt": "step 1"},
		map[string]any{"prompt": "step 2 with {previous}"},
		map[string]any{"prompt": "step 3 with {previous}"},
	}

	result, err := runChain(context.Background(), agent, chain, "")
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "processed: step 3 with processed: step 2 with processed: step 1", result.Content)
}

func TestRunChain_StopsOnError(t *testing.T) {
	original := testRunSubagent
	defer func() { testRunSubagent = original }()

	callCount := 0
	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string) (string, error) {
		callCount++
		if callCount == 2 {
			return "", errors.New("step 2 failed")
		}

		return "ok: " + prompt, nil
	}

	agent := &AgentDef{Name: "test"}
	chain := []any{
		map[string]any{"prompt": "step 1"},
		map[string]any{"prompt": "step 2"},
		map[string]any{"prompt": "step 3"},
	}

	result, err := runChain(context.Background(), agent, chain, "")
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "Chain step 2 failed")
	assert.Equal(t, 2, callCount) // Should have stopped after step 2
}

func TestRunChain_EmptyChain(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	result, err := runChain(context.Background(), agent, []any{}, "")
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Empty(t, result.Content)
}

func TestRunChain_InvalidTaskType(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	chain := []any{
		"not an object",
	}

	result, err := runChain(context.Background(), agent, chain, "")
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "expected object")
}

func TestExtractPrompts_Valid(t *testing.T) {
	items := []any{
		map[string]any{"prompt": "task 1"},
		map[string]any{"prompt": "task 2"},
	}

	prompts, err := extractPrompts(items)
	require.NoError(t, err)
	assert.Equal(t, []string{"task 1", "task 2"}, prompts)
}

func TestExtractPrompts_MissingPrompt(t *testing.T) {
	items := []any{
		map[string]any{"prompt": "task 1"},
		map[string]any{"other": "task 2"},
	}

	_, err := extractPrompts(items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing \"prompt\" field")
}

func TestExtractPrompts_NotString(t *testing.T) {
	items := []any{
		map[string]any{"prompt": 123},
	}

	_, err := extractPrompts(items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestExtractPrompts_EmptyPrompt(t *testing.T) {
	items := []any{
		map[string]any{"prompt": ""},
	}

	_, err := extractPrompts(items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestExtractPrompts_NotObject(t *testing.T) {
	items := []any{
		"not an object",
	}

	_, err := extractPrompts(items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected object")
}

func TestExtractPrompts_TypedArray(t *testing.T) {
	items := []any{
		map[string]any{"prompt": "from map"},
	}

	prompts, err := extractPrompts(items)
	require.NoError(t, err)
	assert.Equal(t, []string{"from map"}, prompts)
}

func TestToAnySlice_AnySlice(t *testing.T) {
	input := []any{1, 2, 3}
	result, ok := toAnySlice(input)
	require.True(t, ok)
	assert.Equal(t, []any{1, 2, 3}, result)
}

func TestToAnySlice_MapSlice(t *testing.T) {
	input := []map[string]any{{"a": 1}, {"b": 2}}
	result, ok := toAnySlice(input)
	require.True(t, ok)
	assert.Len(t, result, 2)
}

func TestToAnySlice_Invalid(t *testing.T) {
	_, ok := toAnySlice("not a slice")
	assert.False(t, ok)
}
