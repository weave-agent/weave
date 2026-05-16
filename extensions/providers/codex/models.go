package codex

import (
	"weave/sdk/model"
)

func init() {
	RegisterModels()
}

// RegisterModels registers Codex model definitions in the global model registry.
func RegisterModels() {
	model.RegisterModel(model.ModelDef{
		ID: "gpt-5.5", Provider: "codex",
		DisplayName: "Codex GPT-5.5", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 1050000, MaxTokens: 128000, Default: true,
	})
	model.RegisterModel(model.ModelDef{
		ID: "gpt-5.4", Provider: "codex",
		DisplayName: "Codex GPT-5.4", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 272000, MaxTokens: 128000,
	})
	model.RegisterModel(model.ModelDef{
		ID: "gpt-5.2", Provider: "codex",
		DisplayName: "Codex GPT-5.2", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 400000, MaxTokens: 128000,
	})
	model.RegisterModel(model.ModelDef{
		ID: "o4-mini", Provider: "codex",
		DisplayName: "Codex o4-mini", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 100000,
	})
	model.RegisterModel(model.ModelDef{
		ID: "o3", Provider: "codex",
		DisplayName: "Codex o3", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 100000,
	})
}
