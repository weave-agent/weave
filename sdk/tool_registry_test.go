package sdk

import (
	"context"
	"errors"
	"sort"
	"testing"
)

type mockTool struct {
	name string
	def  ToolDef
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Definition() ToolDef { return m.def }
func (m *mockTool) Execute(_ context.Context, _ map[string]any) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

func TestRegisterAndRetrieveTool(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("bash", func(Config) (Tool, error) {
		return &mockTool{name: "bash", def: ToolDef{Name: "bash", Description: "run commands"}}, nil
	})

	got, err := GetTool("bash", nil)
	if err != nil {
		t.Fatalf("GetTool: %v", err)
	}

	if got.Name() != "bash" {
		t.Errorf("tool name = %q, want %q", got.Name(), "bash")
	}
}

func TestDuplicateToolRegistration(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("dup", func(Config) (Tool, error) {
		return &mockTool{name: "first"}, nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate tool registration")
		}
	}()

	RegisterTool("dup", func(Config) (Tool, error) {
		return &mockTool{name: "second"}, nil
	})
}

func TestMissingTool(t *testing.T) {
	ResetToolRegistry()

	_, err := GetTool("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestGetTool_FactoryError(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("fail", func(Config) (Tool, error) {
		return nil, errors.New("factory error")
	})

	_, err := GetTool("fail", nil)
	if err == nil {
		t.Fatal("expected error from failing factory")
	}

	if err.Error() != "factory error" {
		t.Errorf("error = %q, want %q", err.Error(), "factory error")
	}
}

func TestListTools(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("bash", func(Config) (Tool, error) {
		return &mockTool{name: "bash"}, nil
	})
	RegisterTool("file", func(Config) (Tool, error) {
		return &mockTool{name: "file"}, nil
	})

	names := ListTools()
	sort.Strings(names)

	want := []string{"bash", "file"}
	if len(names) != len(want) {
		t.Fatalf("ListTools() = %v, want %v", names, want)
	}

	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestResetToolRegistry(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("temp", func(Config) (Tool, error) {
		return &mockTool{name: "temp"}, nil
	})

	ResetToolRegistry()

	names := ListTools()
	if len(names) != 0 {
		t.Errorf("after reset, ListTools() = %v, want empty", names)
	}
}
