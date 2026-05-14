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
		DisplayName: "Kimi For Coding", Reasoning: false, SupportsXHigh: false,
		ContextWindow: 262144, MaxTokens: 32768, Default: true,
	})
}
