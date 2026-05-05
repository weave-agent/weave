package model

func init() { //nolint:gochecknoinits // required to populate model registry before extensions access it
	RegisterBuiltinModels()
}

// RegisterBuiltinModels registers the curated model entries for built-in providers.
func RegisterBuiltinModels() {
	// Anthropic
	RegisterModel(ModelDef{
		ID: "claude-opus-4-7", Provider: ProviderAnthropic,
		DisplayName: "Claude Opus 4.7", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 1000000, MaxTokens: 128000,
	})
	RegisterModel(ModelDef{
		ID: "claude-sonnet-4-6", Provider: ProviderAnthropic,
		DisplayName: "Claude Sonnet 4.6", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 1000000, MaxTokens: 64000, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "claude-opus-4-5", Provider: ProviderAnthropic,
		DisplayName: "Claude Opus 4.5", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 200000, MaxTokens: 64000,
	})
	RegisterModel(ModelDef{
		ID: "claude-sonnet-4-5", Provider: ProviderAnthropic,
		DisplayName: "Claude Sonnet 4.5", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 200000, MaxTokens: 64000,
	})
	RegisterModel(ModelDef{
		ID: "claude-haiku-4-5", Provider: ProviderAnthropic,
		DisplayName: "Claude Haiku 4.5", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 200000, MaxTokens: 64000,
	})
	// OpenAI
	RegisterModel(ModelDef{
		ID: "gpt-5.5", Provider: ProviderOpenAI,
		DisplayName: "GPT-5.5", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 1050000, MaxTokens: 128000, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "gpt-5.4", Provider: ProviderOpenAI,
		DisplayName: "GPT-5.4", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 272000, MaxTokens: 128000,
	})
	RegisterModel(ModelDef{
		ID: "gpt-5.2", Provider: ProviderOpenAI,
		DisplayName: "GPT-5.2", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 400000, MaxTokens: 128000,
	})
	RegisterModel(ModelDef{
		ID: "gpt-4.1", Provider: ProviderOpenAI,
		DisplayName: "GPT-4.1", ContextWindow: 1047576, MaxTokens: 32768,
	})
	RegisterModel(ModelDef{
		ID: "o4-mini", Provider: ProviderOpenAI,
		DisplayName: "o4-mini", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 100000,
	})
	RegisterModel(ModelDef{
		ID: "o3", Provider: ProviderOpenAI,
		DisplayName: "o3", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 100000,
	})
	// ZAI
	RegisterModel(ModelDef{
		ID: "glm-5.1", Provider: "zai",
		DisplayName: "GLM-5.1", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 131072, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "glm-5", Provider: "zai",
		DisplayName: "GLM-5", Reasoning: true,
		ContextWindow: 204800, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.7", Provider: "zai",
		DisplayName: "GLM-4.7", Reasoning: true,
		ContextWindow: 204800, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.7-flash", Provider: "zai",
		DisplayName: "GLM-4.7 Flash", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.7-flashx", Provider: "zai",
		DisplayName: "GLM-4.7 FlashX", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.5-air", Provider: "zai",
		DisplayName: "GLM-4.5 Air", Reasoning: true,
		ContextWindow: 131072, MaxTokens: 98304,
	})
}
