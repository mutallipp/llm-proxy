package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/llm"
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
	if binding.Model == nil || binding.Model.Settings == nil {
		return nil, fmt.Errorf("%w: adapter %q model %q is missing binding model", biz.ErrInternal, adapterConfig.Name, req.Model)
	}

	protocol := string(req.APIFormat)
	associations, ok := binding.Model.Settings.ProtocolPools[protocol]
	if !ok {
		return nil, fmt.Errorf("%w: model %q has no protocol pool for %q", biz.ErrInvalidModel, req.Model, protocol)
	}

	candidates := make([]*ChannelModelsCandidate, 0, len(associations))
	for _, association := range associations {
		if association == nil || association.Disabled || association.Type != "channel_model" || association.ChannelModel == nil || strings.TrimSpace(association.ChannelModel.ModelID) == "" || !targetSupportsAssociation(association, req) {
			continue
		}
		channel := s.ChannelService.GetEnabledChannel(association.ChannelModel.ChannelID)
		if channel == nil || !hasOutboundEndpoint(channel, protocol) {
			continue
		}
		candidates = append(candidates, &ChannelModelsCandidate{Channel: channel, Priority: association.Priority, Models: []biz.ChannelModelEntry{{RequestModel: req.Model, ActualModel: association.ChannelModel.ModelID, Source: "adapter"}}, APIFormat: protocol})
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

func targetSupportsAssociation(association *objects.ModelAssociation, req *llm.Request) bool {
	// ModelAssociation 当前没有目标能力声明，不能伪造能力或回退旧模型组。
	return true
}

func hasModality(modalities []string, expected string) bool {
	for _, modality := range modalities {
		if strings.EqualFold(modality, expected) {
			return true
		}
	}

	return false
}
