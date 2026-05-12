package kimi

import (
	"weave/sdk/model"
)

func init() {
	RegisterModels()
}

// RegisterModels registers Kimi model definitions in the global model registry.
func RegisterModels() {
	model.RegisterModel(model.ModelDef{
		ID: "kimi-for-coding", Provider: "kimi",
		DisplayName: "Kimi For Coding", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 262144, MaxTokens: 32768, Default: true,
	})
	model.RegisterModel(model.ModelDef{
		ID: "k2p6", Provider: "kimi",
		DisplayName: "Kimi K2.6", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 262144, MaxTokens: 32768,
	})
	model.RegisterModel(model.ModelDef{
		ID: "kimi-k2-thinking", Provider: "kimi",
		DisplayName: "Kimi K2 Thinking", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 262144, MaxTokens: 32768,
	})
}
