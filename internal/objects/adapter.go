package objects

import "time"

// AdapterTargetCapabilities 描述适配器目标显式声明的能力。
// 零值表示目标未声明该能力，因此不会被能力过滤视为支持。
type AdapterTargetCapabilities struct {
	SupportsTools     bool `json:"supports_tools"`
	SupportsStream    bool `json:"supports_stream"`
	SupportsReasoning bool `json:"supports_reasoning"`
	ContextLength     int  `json:"context_length,omitempty"`
	MaxOutputTokens   int  `json:"max_output_tokens,omitempty"`
	// StreamPolicy 目标级流式响应策略："unlimited"（跟随下游）、"require"（强制流式）、"forbid"（禁止流式）。
	// 空字符串表示旧数据，按 supports_stream bool 兼容处理。
	StreamPolicy     string   `json:"stream_policy,omitempty"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

// RuntimeAdapter 是消费请求使用的不可变适配器运行时配置。
type RuntimeAdapter struct {
	ID               int
	Name             string
	DisplayName      string
	InboundAPIFormat string
	Bindings         map[string]*RuntimeAdapterBinding
	BindingOrder     []string
}

// RuntimeAdapterBinding 将消费端逻辑模型绑定到不可变模型快照。
type RuntimeAdapterBinding struct {
	ID            int
	SourceModelID string
	Model         *RuntimeModel
	Enabled       bool
}

// RuntimeModel 是适配器运行时使用的逻辑模型及其协议池快照。
type RuntimeModel struct {
	ID       int
	ModelID  string
	Name     string
	Settings *ModelSettings
}

// AdapterDiagnostic 描述快照构建时被排除的目标或配置问题。
type AdapterDiagnostic struct {
	AdapterName   string
	SourceModelID string
	TargetID      int
	ChannelID     int
	TargetModelID string
	Reason        string
}

// AdapterSnapshot 是一次完整的适配器运行时快照。
// Adapters、map 和切片在发布后只读，刷新通过一次原子替换整体生效。
type AdapterSnapshot struct {
	Version                 uint64
	RefreshedAt             time.Time
	LastSuccessfulRefreshAt time.Time
	Adapters                map[string]*RuntimeAdapter
	Diagnostics             []AdapterDiagnostic
}

// AdapterRuntimeStatus 描述最近一次刷新结果，失败时不会影响当前快照。
type AdapterRuntimeStatus struct {
	SnapshotVersion         uint64
	RefreshedAt             time.Time
	LastSuccessfulRefreshAt time.Time
	LastRefreshError        string
}
