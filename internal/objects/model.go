package objects

import "fmt"

type ModelCardReasoning struct {
	Supported bool `json:"supported"`
	Default   bool `json:"default"`
}

type ModelCardModalities struct {
	// "text","image","video"
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type ModelCardCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

type ModelCardLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type ModelCard struct {
	Reasoning   ModelCardReasoning  `json:"reasoning"`
	ToolCall    bool                `json:"toolCall"`
	Temperature bool                `json:"temperature"`
	Modalities  ModelCardModalities `json:"modalities"`
	Vision      bool                `json:"vision"`
	Cost        ModelCardCost       `json:"cost"`
	Limit       ModelCardLimit      `json:"limit"`
	Knowledge   string              `json:"knowledge"`
	ReleaseDate string              `json:"releaseDate"`
	LastUpdated string              `json:"lastUpdated"`
}

type ModelSettings struct {
	DisableDeveloperSettingsInheritance bool                           `json:"disableDeveloperSettingsInheritance"`
	ProtocolPools                       map[string][]*ModelAssociation `json:"protocolPools,omitempty"`
}

// SupportedInboundAPIFormats 是协议池允许的入站协议。协议池 key 同时决定出站协议。
var SupportedInboundAPIFormats = map[string]struct{}{
	"openai":    {},
	"anthropic": {},
}

// IsSupportedInboundAPIFormat 判断协议池 key 是否在当前支持的协议族白名单内。
func IsSupportedInboundAPIFormat(protocol string) bool {
	_, ok := SupportedInboundAPIFormats[protocol]

	return ok
}

// ValidateProtocolPools 校验协议池结构，避免旧 settings 被静默转换或写入非法协议。
func (s *ModelSettings) ValidateProtocolPools() error {
	if s == nil || s.ProtocolPools == nil {
		return nil
	}
	for protocol, associations := range s.ProtocolPools {
		seen := make(map[ChannelModelKey]struct{})
		if _, ok := SupportedInboundAPIFormats[protocol]; !ok {
			return fmt.Errorf("unsupported protocol pool %q", protocol)
		}
		for _, association := range associations {
			if association == nil || association.Disabled || association.ChannelModel == nil {
				continue
			}
			key := ChannelModelKey{ChannelID: association.ChannelModel.ChannelID, ModelID: association.ChannelModel.ModelID}
			if _, ok := seen[key]; ok {
				return fmt.Errorf("duplicate channel model association in protocol pools: %d:%s", key.ChannelID, key.ModelID)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

// ChannelModelKey 用于对象层校验，避免引入业务 matcher 包。
type ChannelModelKey struct {
	ChannelID int
	ModelID   string
}

const (
	ModelAssociationConditionFieldPromptTokens        = "prompt_tokens"
	ModelAssociationConditionFieldStream              = "stream"
	ModelAssociationConditionFieldRequestFormat       = "request_format"
	ModelAssociationConditionFieldDailyTime           = "daily_time"
	ModelAssociationConditionFieldHasImage            = "has_image"
	ModelAssociationConditionFieldHasVideo            = "has_video"
	ModelAssociationConditionFieldHasDocument         = "has_document"
	ModelAssociationConditionFieldHasAudio            = "has_audio"
	ModelAssociationConditionFieldRequestHeader       = "request_header"
	ModelAssociationConditionFieldRequestHeaderPrefix = "request_header."
)

type ModelAssociation struct {
	// channel_model: the specified model id in the specified channel
	// channel_regex: the specified pattern in the specified channel
	// regex: the pattern for all channels
	// model: the specified model id
	// channel_tags_model: the specified model id in channels with specified tags (OR logic)
	// channel_tags_regex: the specified pattern in channels with specified tags (OR logic)
	Type             string                       `json:"type"`
	Priority         int                          `json:"priority"` // Lower value = higher priority, default 0
	Disabled         bool                         `json:"disabled"`
	Auto             bool                         `json:"auto,omitempty"`
	DisabledReason   string                       `json:"disabledReason,omitempty"`
	When             *ModelAssociationWhen        `json:"when,omitempty"`
	ChannelModel     *ChannelModelAssociation     `json:"channelModel"`
	ChannelRegex     *ChannelRegexAssociation     `json:"channelRegex"`
	Regex            *RegexAssociation            `json:"regex"`
	ModelID          *ModelIDAssociation          `json:"modelId"`
	ChannelTagsModel *ChannelTagsModelAssociation `json:"channelTagsModel"`
	ChannelTagsRegex *ChannelTagsRegexAssociation `json:"channelTagsRegex"`
}

type ModelAssociationWhen struct {
	Enabled   bool       `json:"enabled"`
	Condition *Condition `json:"condition,omitempty"`
}

type ExcludeAssociation struct {
	ChannelNamePattern string   `json:"channelNamePattern"`
	ChannelIds         []int    `json:"channelIds"`
	ChannelTags        []string `json:"channelTags"`
}

type ChannelModelAssociation struct {
	ChannelID int    `json:"channelId"`
	ModelID   string `json:"modelId"`
}

type ChannelRegexAssociation struct {
	ChannelID int    `json:"channelId"`
	Pattern   string `json:"pattern"`
}

type RegexAssociation struct {
	Pattern string                `json:"pattern"`
	Exclude []*ExcludeAssociation `json:"exclude"`
}

type ModelIDAssociation struct {
	ModelID string                `json:"modelId"`
	Exclude []*ExcludeAssociation `json:"exclude"`
}

type ChannelTagsModelAssociation struct {
	ChannelTags []string `json:"channelTags"`
	ModelID     string   `json:"modelId"`
}

type ChannelTagsRegexAssociation struct {
	ChannelTags []string `json:"channelTags"`
	Pattern     string   `json:"pattern"`
}
