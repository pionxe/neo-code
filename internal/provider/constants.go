package provider

// Driver 与 OpenAI-compatible 协议常量用于在 config/provider 间共享稳定枚举值，避免字面量漂移。
const (
	DriverOpenAICompat = "openaicompat"
	DriverGemini       = "gemini"
	DriverAnthropic    = "anthropic"

	DiscoveryEndpointPathModels = "/models"
)
