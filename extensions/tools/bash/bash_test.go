package bash

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSandboxer is a minimal Sandboxer implementation for testing.
type testSandboxer struct {
	wrapFn func(cmd, dir string) (string, error)
}

func (ts *testSandboxer) WrapCommand(cmd, dir string) (string, error) { return ts.wrapFn(cmd, dir) }
func (ts *testSandboxer) AllowWrite(path string) bool                 { return true }
func (ts *testSandboxer) AllowRead(path string) bool                  { return true }
func (ts *testSandboxer) Mode() string                                { return "auto" }
func (ts *testSandboxer) SetMode(string)                              {}

func TestRegister(t *testing.T) {
	tool, err := sdk.GetTool("bash", nil)
	require.NoError(t, err)
	assert.Equal(t, "bash", tool.Name())
}

func TestDefinition(t *testing.T) {
	tool := &tool{}
	def := tool.Definition()
	assert.Equal(t, "bash", def.Name)
	assert.NotNil(t, def.Parameters)
}

func TestDirFromConfig(t *testing.T) {
	t.Run("resolves project root from .weave/settings.json", func(t *testing.T) {
		cfg := sdk.FilePathConfig("/project/.weave/settings.json")
		dir := dirFromConfig(cfg)
		assert.Equal(t, "/project", dir)
	})

	t.Run("resolves plain settings.json path", func(t *testing.T) {
		cfg := sdk.FilePathConfig("/project/settings.json")
		dir := dirFromConfig(cfg)
		assert.Equal(t, "/project", dir)
	})

	t.Run("falls back to cwd when FilePath empty", func(t *testing.T) {
		cfg := sdk.FilePathConfig("")
		dir := dirFromConfig(cfg)
		assert.NotEmpty(t, dir)
	})
}

func TestExecute(t *testing.T) {
	tool := &tool{}

	tests := []struct {
		name      string
		args      map[string]any
		wantError bool
		check     func(t *testing.T, result sdk.ToolResult)
	}{
		{
			name:      "missing command",
			args:      map[string]any{},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "command is required")
			},
		},
		{
			name:      "empty command",
			args:      map[string]any{"command": ""},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "command is required")
			},
		},
		{
			name:      "simple echo",
			args:      map[string]any{"command": "echo hello"},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "hello")
			},
		},
		{
			name:      "failure exit code",
			args:      map[string]any{"command": "exit 1"},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "[exit code 1]")
			},
		},
		{
			name:      "stderr captured",
			args:      map[string]any{"command": "echo err >&2"},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "err")
			},
		},
		{
			name: "timeout",
			args: map[string]any{
				"command": "sleep 10",
				"timeout": float64(1),
			},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "error:")
			},
		},
		{
			name:      "empty output",
			args:      map[string]any{"command": "true"},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Empty(t, result.Content)
			},
		},
		{
			name:      "large output truncation",
			args:      map[string]any{"command": "seq 3000"},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "output truncated")
			},
		},
		{
			name:      "command with args",
			args:      map[string]any{"command": "echo -n 'no newline'"},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Equal(t, "no newline", result.Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := tool.Execute(ctx, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantError, result.IsError)

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestExecuteCanceled(t *testing.T) {
	tool := &tool{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := tool.Execute(ctx, map[string]any{"command": "sleep 10"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "canceled")
}

func TestExecuteTruncation(t *testing.T) {
	tool := &tool{}
	// Generate enough lines to exceed the 2000-line default
	largeCmd := "for i in $(seq 1 3000); do echo \"line $i\"; done"
	result, err := tool.Execute(context.Background(), map[string]any{"command": largeCmd})
	require.NoError(t, err)

	lines := strings.Split(result.Content, "\n")
	assert.LessOrEqual(t, len(lines), 2010) // 2000 lines + truncation notice
}

// recordingBus is a test helper that records all published events.
type recordingBus struct {
	events []sdk.Event
	mu     sync.Mutex
}

func (r *recordingBus) Publish(e sdk.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, e)
}

func (r *recordingBus) On(topic string, h sdk.Handler) {}

func (r *recordingBus) OnAll(h sdk.Handler) {}

func (r *recordingBus) Off(h sdk.Handler) {}

func (r *recordingBus) Close() error { return nil }

func (r *recordingBus) Events() []sdk.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]sdk.Event(nil), r.events...)
}

func TestExecuteStreaming(t *testing.T) {
	tool := &tool{}

	t.Run("publishes stdout events", func(t *testing.T) {
		bus := &recordingBus{}
		ctx := sdk.WithBus(context.Background(), bus)

		result, err := tool.Execute(ctx, map[string]any{"command": "echo hello"})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "hello")

		events := bus.Events()

		var outputEvents []sdk.Event

		for _, e := range events {
			if e.Topic == "tool.bash.output" {
				outputEvents = append(outputEvents, e)
			}
		}

		require.Len(t, outputEvents, 1)

		payload, ok := outputEvents[0].Payload.(BashOutputPayload)
		require.True(t, ok)
		assert.Equal(t, "echo hello", payload.Command)
		assert.Equal(t, "hello", payload.Line)
		assert.Equal(t, "stdout", payload.Stream)
	})

	t.Run("publishes stderr events", func(t *testing.T) {
		bus := &recordingBus{}
		ctx := sdk.WithBus(context.Background(), bus)

		result, err := tool.Execute(ctx, map[string]any{"command": "echo err >&2"})
		require.NoError(t, err)
		assert.Contains(t, result.Content, "err")

		events := bus.Events()

		var outputEvents []sdk.Event

		for _, e := range events {
			if e.Topic == "tool.bash.output" {
				outputEvents = append(outputEvents, e)
			}
		}

		require.Len(t, outputEvents, 1)

		payload, ok := outputEvents[0].Payload.(BashOutputPayload)
		require.True(t, ok)
		assert.Equal(t, "stderr", payload.Stream)
		assert.Equal(t, "err", payload.Line)
	})

	t.Run("publishes multiple lines in order", func(t *testing.T) {
		bus := &recordingBus{}
		ctx := sdk.WithBus(context.Background(), bus)

		result, err := tool.Execute(ctx, map[string]any{"command": "echo a && echo b && echo c"})
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(result.Content), "\n")
		assert.Equal(t, []string{"a", "b", "c"}, lines)

		events := bus.Events()

		var outputEvents []sdk.Event

		for _, e := range events {
			if e.Topic == "tool.bash.output" {
				outputEvents = append(outputEvents, e)
			}
		}

		require.Len(t, outputEvents, 3)

		for i, expected := range []string{"a", "b", "c"} {
			payload := outputEvents[i].Payload.(BashOutputPayload)
			assert.Equal(t, expected, payload.Line)
			assert.Equal(t, "stdout", payload.Stream)
		}
	})

	t.Run("no events when bus is nil", func(t *testing.T) {
		// context without bus
		result, err := tool.Execute(context.Background(), map[string]any{"command": "echo hello"})
		require.NoError(t, err)
		assert.Contains(t, result.Content, "hello")
	})
}

func TestExecuteStreamingTimeout(t *testing.T) {
	tool := &tool{}

	t.Run("returns partial output on timeout", func(t *testing.T) {
		bus := &recordingBus{}
		ctx := sdk.WithBus(context.Background(), bus)

		// Write some output, then sleep past timeout
		result, err := tool.Execute(ctx, map[string]any{
			"command": "echo before && sleep 10",
			"timeout": float64(1),
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content, "before")
		assert.Contains(t, result.Content, "timed out")

		events := bus.Events()

		var outputEvents []sdk.Event

		for _, e := range events {
			if e.Topic == "tool.bash.output" {
				outputEvents = append(outputEvents, e)
			}
		}

		require.Len(t, outputEvents, 1)
		assert.Equal(t, "before", outputEvents[0].Payload.(BashOutputPayload).Line)
	})
}

func TestExecuteWithSandboxer(t *testing.T) {
	orig := sdk.GetSandboxer()

	sdk.SetSandboxer(nil)
	t.Cleanup(func() { sdk.SetSandboxer(orig) })

	tool := &tool{dir: "/test/dir"}

	t.Run("nil sandboxer passes command through", func(t *testing.T) {
		sdk.SetSandboxer(nil)

		result, err := tool.Execute(context.Background(), map[string]any{"command": "echo untouched"})
		require.NoError(t, err)
		assert.Contains(t, result.Content, "untouched")
		assert.False(t, result.IsError)
	})

	t.Run("sandboxer wraps command", func(t *testing.T) {
		var mu sync.Mutex

		gotCmd, gotDir := "", ""

		s := &testSandboxer{
			wrapFn: func(cmd, dir string) (string, error) {
				mu.Lock()
				gotCmd, gotDir = cmd, dir
				mu.Unlock()

				return cmd, nil
			},
		}
		sdk.SetSandboxer(s)

		result, err := tool.Execute(context.Background(), map[string]any{"command": "echo wrapped"})
		require.NoError(t, err)
		assert.Contains(t, result.Content, "wrapped")

		mu.Lock()
		assert.Equal(t, "echo wrapped", gotCmd)
		assert.Equal(t, "/test/dir", gotDir)
		mu.Unlock()
	})

	t.Run("sandboxer error returns sandbox error", func(t *testing.T) {
		s := &testSandboxer{
			wrapFn: func(cmd, dir string) (string, error) {
				return "", errors.New("sandbox unavailable")
			},
		}
		sdk.SetSandboxer(s)

		result, err := tool.Execute(context.Background(), map[string]any{"command": "echo fail"})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content, "sandbox: sandbox unavailable")
	})
}
