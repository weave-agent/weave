package sdk

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
