package sdk

//go:generate moq -fmt goimports -stub -out tool_mock_test.go . Tool

import "context"

type ToolDef struct {
	Name        string
	Description string
	Parameters  any
}

type Tool interface {
	Name() string
	Definition() ToolDef
	Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}

type ToolResult struct {
	Content string
	IsError bool
}
