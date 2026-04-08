package provider

// CapabilitySupport 表示模型能力的确认状态。
type CapabilitySupport string

const (
	CapabilitySupportUnknown     CapabilitySupport = "unknown"
	CapabilitySupportUnsupported CapabilitySupport = "unsupported"
	CapabilitySupportSupported   CapabilitySupport = "supported"
)

// ReasoningMode 表示模型推理能力的形态。
type ReasoningMode string

const (
	ReasoningModeUnknown      ReasoningMode = "unknown"
	ReasoningModeNone         ReasoningMode = "none"
	ReasoningModeNative       ReasoningMode = "native"
	ReasoningModeConfigurable ReasoningMode = "configurable"
)

// DriverCapabilities 描述 driver 层本身是否能传输某类能力。
type DriverCapabilities struct {
	Streaming           bool
	ToolTransport       bool
	ModelDiscovery      bool
	ImageInputTransport bool
}

// ModelCapabilities 描述具体模型的能力声明。
type ModelCapabilities struct {
	ToolCalling      CapabilitySupport
	ImageInput       CapabilitySupport
	ReasoningMode    ReasoningMode
	MaxContextTokens int
	MaxOutputTokens  int
}

// ResolvedCapabilities 是 driver 能力与模型能力的合并视图。
type ResolvedCapabilities struct {
	Driver DriverCapabilities
	Model  ModelCapabilities
}
