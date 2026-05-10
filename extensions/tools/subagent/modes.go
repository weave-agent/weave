package subagent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"weave/sdk"
)

// runParallel executes multiple subagent tasks concurrently and aggregates results.
func runParallel(ctx context.Context, agent *AgentDef, tasks []any, cwd string, broker *Broker) (sdk.ToolResult, error) {
	prompts, err := extractPrompts(tasks)
	if err != nil {
		//nolint:nilerr // tool protocol: errors in Content, not return
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	if len(prompts) == 0 {
		return sdk.ToolResult{Content: ""}, nil
	}

	type result struct {
		index  int
		output string
		err    error
	}

	results := make([]result, len(prompts))

	const maxConcurrent = 5

	type task struct {
		idx    int
		prompt string
	}

	taskCh := make(chan task, len(prompts))
	for i, p := range prompts {
		taskCh <- task{idx: i, prompt: p}
	}

	close(taskCh)

	var wg sync.WaitGroup

	workerCount := min(maxConcurrent, len(prompts))

	for range workerCount {
		wg.Go(func() {
			for t := range taskCh {
				if ctx.Err() != nil {
					results[t.idx] = result{index: t.idx, err: ctx.Err()}

					continue
				}

				var subagentID string
				if agent.Messaging {
					subagentID = generateAgentID(agent.Name)
				}

				output, err := runSubagent(ctx, agent, t.prompt, cwd, subagentID, broker)
				results[t.idx] = result{index: t.idx, output: output, err: err}
			}
		})
	}

	wg.Wait()

	var sb strings.Builder

	var hasErrors bool

	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n\n---\n\n")
		}

		fmt.Fprintf(&sb, "Task %d:\n", i+1)

		if r.err != nil {
			hasErrors = true

			fmt.Fprintf(&sb, "ERROR: %v\n", r.err)
		} else {
			sb.WriteString(r.output)
		}
	}

	return sdk.ToolResult{
		Content: sb.String(),
		IsError: hasErrors,
	}, nil
}

// runChain executes subagent tasks sequentially, substituting {previous} with prior result.
func runChain(ctx context.Context, agent *AgentDef, chain []any, cwd string, broker *Broker) (sdk.ToolResult, error) {
	prompts, err := extractPrompts(chain)
	if err != nil {
		//nolint:nilerr // tool protocol: errors in Content, not return
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	if len(prompts) == 0 {
		return sdk.ToolResult{Content: ""}, nil
	}

	var previous string

	for i, prompt := range prompts {
		prompt = strings.ReplaceAll(prompt, "{previous}", previous)

		var subagentID string
		if agent.Messaging {
			subagentID = generateAgentID(agent.Name)
		}

		output, err := runSubagent(ctx, agent, prompt, cwd, subagentID, broker)
		if err != nil {
			return sdk.ToolResult{
				Content: fmt.Sprintf("Chain step %d failed: %v", i+1, err),
				IsError: true,
			}, nil
		}

		previous = output
	}

	return sdk.ToolResult{Content: previous}, nil
}

// extractPrompts extracts prompt strings from an array of task/step specs.
// Each element should be a map with a "prompt" string field.
func extractPrompts(items []any) ([]string, error) {
	prompts := make([]string, 0, len(items))

	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("task %d: expected object, got %T", i+1, item)
		}

		promptVal, ok := m["prompt"]
		if !ok {
			return nil, fmt.Errorf("task %d: missing \"prompt\" field", i+1)
		}

		prompt, ok := promptVal.(string)
		if !ok {
			return nil, fmt.Errorf("task %d: \"prompt\" must be a string, got %T", i+1, promptVal)
		}

		if prompt == "" {
			return nil, fmt.Errorf("task %d: \"prompt\" cannot be empty", i+1)
		}

		prompts = append(prompts, prompt)
	}

	return prompts, nil
}

// toAnySlice normalizes array values from JSON decoding to []any.
func toAnySlice(v any) ([]any, bool) {
	if arr, ok := v.([]any); ok {
		return arr, true
	}

	if arr, ok := v.([]map[string]any); ok {
		out := make([]any, len(arr))
		for i, m := range arr {
			out[i] = m
		}

		return out, true
	}

	return nil, false
}
