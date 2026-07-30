package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
)

// AdapterCandidateSelector 只在当前适配器快照的目标池内生成候选。
type AdapterCandidateSelector struct {
	AdapterService *biz.AdapterService
	ChannelService *biz.ChannelService
}

func NewAdapterCandidateSelector(adapterService *biz.AdapterService, channelService *biz.ChannelService) *AdapterCandidateSelector {
	return &AdapterCandidateSelector{
		AdapterService: adapterService,
		ChannelService: channelService,
	}
}

// Select 将逻辑模型解析为目标模型，并严格使用目标声明的出站协议。
// 缺少适配器上下文时直接报内部配置错误，绝不回退到全渠道选择。
func (s *AdapterCandidateSelector) Select(ctx context.Context, req *llm.Request) ([]*ChannelModelsCandidate, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: adapter request is nil", biz.ErrInternal)
	}

	adapterConfig, err := s.resolveAdapter(ctx)
	if err != nil {
		return nil, err
	}
	if adapterConfig.InboundAPIFormat == "" || string(req.APIFormat) != adapterConfig.InboundAPIFormat {
		return nil, fmt.Errorf("%w: adapter %q requires inbound api format %q, got %q", biz.ErrInvalidModel, adapterConfig.Name, adapterConfig.InboundAPIFormat, req.APIFormat)
	}

	binding, ok := adapterConfig.Bindings[req.Model]
	if !ok || binding == nil || !binding.Enabled {
		return nil, fmt.Errorf("%w: model %q is not bound to adapter %q", biz.ErrInvalidModel, req.Model, adapterConfig.Name)
	}
	if binding.ModelGroup == nil {
		return nil, fmt.Errorf("%w: adapter %q model %q has no model group", biz.ErrInternal, adapterConfig.Name, req.Model)
	}

	protocol := binding.ModelGroup.Protocols[adapterConfig.InboundAPIFormat]
	if protocol == nil || protocol.InboundAPIFormat != string(req.APIFormat) {
		return nil, fmt.Errorf("%w: model group %q does not support inbound api format %q", biz.ErrInvalidModel, binding.ModelGroup.Name, req.APIFormat)
	}

	candidates := make([]*ChannelModelsCandidate, 0, len(protocol.Targets))
	for _, target := range protocol.Targets {
		if target == nil || strings.TrimSpace(target.TargetModelID) == "" || strings.TrimSpace(target.OutboundAPIFormat) == "" || !targetSupportsRequest(target, req) {
			continue
		}

		// 每次选择都重新从 ChannelService 读取启用渠道，避免渠道热更新后继续使用失效实例。
		channel := s.ChannelService.GetEnabledChannel(target.ChannelID)
		if channel == nil || !hasOutboundEndpoint(channel, target.OutboundAPIFormat) {
			continue
		}

		candidates = append(candidates, &ChannelModelsCandidate{
			Channel:  channel,
			Priority: target.Priority,
			Models: []biz.ChannelModelEntry{{
				RequestModel: req.Model,
				ActualModel:  target.TargetModelID,
				Source:       "adapter",
			}},
			APIFormat: target.OutboundAPIFormat,
		})
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: no usable target for adapter %q model %q", biz.ErrInvalidModel, adapterConfig.Name, req.Model)
	}

	return candidates, nil
}

func (s *AdapterCandidateSelector) resolveAdapter(ctx context.Context) (*objects.RuntimeAdapter, error) {
	if adapterConfig, ok := contexts.GetRuntimeAdapter(ctx); ok && adapterConfig != nil {
		if adapterConfig.Name == "" {
			return nil, fmt.Errorf("%w: runtime adapter has empty name", biz.ErrInternal)
		}

		return adapterConfig, nil
	}

	if adapterName, ok := contexts.GetAdapterName(ctx); ok {
		return s.AdapterService.Resolve(ctx, adapterName)
	}

	return nil, fmt.Errorf("%w: runtime adapter context is missing", biz.ErrInternal)
}

func hasOutboundEndpoint(channel *biz.Channel, outboundAPIFormat string) bool {
	for _, endpoint := range channel.ResolveEndpoints() {
		if endpoint.APIFormat == outboundAPIFormat {
			return true
		}
	}

	return false
}

func targetSupportsRequest(target *objects.RuntimeModelGroupTarget, req *llm.Request) bool {
	capabilities := target.Capabilities
	if req.Stream != nil && *req.Stream && !capabilities.SupportsStream {
		return false
	}
	if len(req.Tools) > 0 && !capabilities.SupportsTools {
		return false
	}

	features := detectRequestContentFeatures(req)
	if features.hasImage && !hasModality(capabilities.InputModalities, "image") {
		return false
	}
	if features.hasVideo && !hasModality(capabilities.InputModalities, "video") {
		return false
	}
	if features.hasAudio && !hasModality(capabilities.InputModalities, "audio") {
		return false
	}

	switch req.RequestType {
	case llm.RequestTypeImage:
		return hasModality(capabilities.OutputModalities, "image")
	case llm.RequestTypeVideo:
		return hasModality(capabilities.OutputModalities, "video")
	case llm.RequestTypeSpeech:
		return hasModality(capabilities.OutputModalities, "audio")
	case llm.RequestTypeTranscription, llm.RequestTypeTranslation:
		return hasModality(capabilities.InputModalities, "audio")
	default:
		return true
	}
}

func hasModality(modalities []string, expected string) bool {
	for _, modality := range modalities {
		if strings.EqualFold(modality, expected) {
			return true
		}
	}

	return false
}
