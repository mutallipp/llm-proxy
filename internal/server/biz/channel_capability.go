package biz

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/samber/lo"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/model"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

// SaveChannelCapabilitiesInput 是渠道能力声明的写入参数。
type SaveChannelCapabilitiesInput struct {
	ChannelID         objects.GUID
	DeclaredProtocols []string
	Models            []objects.ChannelModelCapability
}

// RevokedChannelCapabilitiesPayload 描述能力撤销后的自动处置与手动提示。
type RevokedChannelCapabilitiesPayload struct {
	AutoDisabledCount int
	ManualNotices     []string
}

// SaveChannelCapabilitiesPayload 是能力保存后的结构化结果。
type SaveChannelCapabilitiesPayload struct {
	Channel         *ent.Channel
	AddedCount      int
	UnmatchedModels []string
	Revoked         *RevokedChannelCapabilitiesPayload
}

// SaveChannelEndpointsPayload 是端点保存及能力复核结果。
type SaveChannelEndpointsPayload struct {
	Channel *ent.Channel
	Revoked *RevokedChannelCapabilitiesPayload
}

// SaveProtocolCapabilities 保存渠道协议能力并触发全量协议池对账。
func (svc *ChannelService) SaveProtocolCapabilities(ctx context.Context, input SaveChannelCapabilitiesInput) (*SaveChannelCapabilitiesPayload, error) {
	channelID := input.ChannelID.ID
	channelEntity, err := svc.entFromContext(ctx).Channel.Get(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}
	if err := validateProtocolCapabilities(channelEntity, input.DeclaredProtocols, input.Models); err != nil {
		return nil, err
	}

	previous := channelEntity.ProtocolCapabilities
	current := mergeProtocolCapabilities(previous, objects.ChannelProtocolCapabilities{
		DeclaredProtocols: input.DeclaredProtocols,
		Models:            input.Models,
	})
	if err := validateMergedProtocolCapabilities(channelEntity, current); err != nil {
		return nil, err
	}

	var updated *ent.Channel
	var reconcileStats AssociationReconcileStats
	err = svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		updated, err = svc.entFromContext(txCtx).Channel.UpdateOneID(channelID).
			SetProtocolCapabilities(current).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("failed to save channel protocol capabilities: %w", err)
		}
		reconcileStats, err = svc.reconcileChannelProtocolCapabilities(txCtx, channelID, previous, current)
		return err
	})
	if err != nil {
		return nil, err
	}
	if ent.TxFromContext(ctx) == nil {
		updated.Unwrap()
	}

	unmatched, err := svc.unmatchedCapabilityModels(ctx, current.Models)
	if err != nil {
		return nil, err
	}
	if reconcileStats.ManualNotices == nil {
		reconcileStats.ManualNotices = []string{}
	}

	return &SaveChannelCapabilitiesPayload{
		Channel:         updated,
		AddedCount:      reconcileStats.AddedCount,
		UnmatchedModels: unmatched,
		Revoked: &RevokedChannelCapabilitiesPayload{
			AutoDisabledCount: reconcileStats.AutoDisabledCount,
			ManualNotices:     reconcileStats.ManualNotices,
		},
	}, nil
}

func validateProtocolCapabilities(ch *ent.Channel, declaredProtocols []string, models []objects.ChannelModelCapability) error {
	seenProtocols := make(map[string]struct{}, len(declaredProtocols))
	for _, protocol := range declaredProtocols {
		if !objects.IsSupportedInboundAPIFormat(protocol) {
			return fmt.Errorf("unsupported protocol capability %q", protocol)
		}
		if _, exists := seenProtocols[protocol]; exists {
			return fmt.Errorf("duplicate protocol capability %q", protocol)
		}
		seenProtocols[protocol] = struct{}{}
	}

	seenModels := make(map[string]struct{}, len(models))
	for _, modelCapability := range models {
		if modelCapability.ModelID == "" {
			return fmt.Errorf("model capability modelId is required")
		}
		if _, exists := seenModels[modelCapability.ModelID]; exists {
			return fmt.Errorf("duplicate model capability %q", modelCapability.ModelID)
		}
		seenModels[modelCapability.ModelID] = struct{}{}
		seenModelProtocols := make(map[string]struct{}, len(modelCapability.Protocols))
		for _, protocol := range modelCapability.Protocols {
			if !objects.IsSupportedInboundAPIFormat(protocol) {
				return fmt.Errorf("unsupported protocol capability %q for model %q", protocol, modelCapability.ModelID)
			}
			if _, exists := seenProtocols[protocol]; !exists {
				return fmt.Errorf("model %q declares protocol %q that the channel does not declare", modelCapability.ModelID, protocol)
			}
			if _, exists := seenModelProtocols[protocol]; exists {
				return fmt.Errorf("duplicate protocol capability %q for model %q", protocol, modelCapability.ModelID)
			}
			seenModelProtocols[protocol] = struct{}{}
		}
	}

	if ch != nil {
		for _, protocol := range declaredProtocols {
			if !channelSupportsProtocolFamily(resolveChannelEndpoints(ch), protocol) {
				return fmt.Errorf("channel endpoints do not support protocol %q", protocol)
			}
		}
	}

	return nil
}

func validateMergedProtocolCapabilities(ch *ent.Channel, capabilities objects.ChannelProtocolCapabilities) error {
	return validateProtocolCapabilities(ch, capabilities.DeclaredProtocols, capabilities.Models)
}

// channelSupportsProtocolFamily 判断端点是否具备协议族的完整端点能力。
func channelSupportsProtocolFamily(resolvedEndpoints []objects.ChannelEndpoint, family string) bool {
	prefix, ok := protocolPoolEndpointAPIPrefixes[family]
	if !ok {
		return false
	}
	for _, endpoint := range resolvedEndpoints {
		if strings.HasPrefix(endpoint.APIFormat, prefix) {
			return true
		}
	}

	return false
}

var protocolPoolEndpointAPIPrefixes = map[string]string{
	"openai":    "openai/",
	"anthropic": "anthropic/",
}

func resolveChannelEndpoints(ch *ent.Channel) []objects.ChannelEndpoint {
	if ch == nil {
		return nil
	}
	return mergeEndpoints(DefaultEndpointsForChannelType(ch.Type), ch.Endpoints)
}

func mergeProtocolCapabilities(previous, submitted objects.ChannelProtocolCapabilities) objects.ChannelProtocolCapabilities {
	previousModels := make(map[string]objects.ChannelModelCapability, len(previous.Models))
	for _, modelCapability := range previous.Models {
		previousModels[modelCapability.ModelID] = cloneChannelModelCapability(modelCapability)
	}
	submittedModelIDs := make(map[string]struct{}, len(submitted.Models))
	for _, modelCapability := range submitted.Models {
		submittedModelIDs[modelCapability.ModelID] = struct{}{}
		previousModels[modelCapability.ModelID] = cloneChannelModelCapability(modelCapability)
	}

	previousDeclared := make(map[string]struct{}, len(previous.DeclaredProtocols))
	for _, protocol := range previous.DeclaredProtocols {
		previousDeclared[protocol] = struct{}{}
	}
	currentDeclared := lo.Uniq(append([]string{}, submitted.DeclaredProtocols...))
	currentDeclaredSet := make(map[string]struct{}, len(currentDeclared))
	for _, protocol := range currentDeclared {
		currentDeclaredSet[protocol] = struct{}{}
	}

	// 未触及的模型保留原声明；渠道新增协议传播到未在本次提交中编辑的模型。
	for modelID, modelCapability := range previousModels {
		if _, edited := submittedModelIDs[modelID]; edited {
			continue
		}
		protocols := make([]string, 0, len(modelCapability.Protocols))
		for _, protocol := range modelCapability.Protocols {
			if _, declared := currentDeclaredSet[protocol]; declared {
				protocols = append(protocols, protocol)
			}
		}
		for _, protocol := range currentDeclared {
			if _, wasDeclared := previousDeclared[protocol]; !wasDeclared {
				protocols = append(protocols, protocol)
			}
		}
		modelCapability.Protocols = lo.Uniq(protocols)
		previousModels[modelID] = modelCapability
	}

	modelIDs := make([]string, 0, len(previousModels))
	for modelID := range previousModels {
		modelIDs = append(modelIDs, modelID)
	}
	sort.Strings(modelIDs)
	models := make([]objects.ChannelModelCapability, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		models = append(models, previousModels[modelID])
	}

	return objects.ChannelProtocolCapabilities{
		DeclaredProtocols: currentDeclared,
		Models:            models,
	}
}

func cloneChannelModelCapability(capability objects.ChannelModelCapability) objects.ChannelModelCapability {
	capability.Protocols = append([]string(nil), capability.Protocols...)
	return capability
}

func capabilitiesAfterModelSync(previous objects.ChannelProtocolCapabilities, addedModels, removedModels []string) objects.ChannelProtocolCapabilities {
	removed := make(map[string]struct{}, len(removedModels))
	for _, modelID := range removedModels {
		removed[modelID] = struct{}{}
	}
	added := make(map[string]struct{}, len(addedModels))
	for _, modelID := range addedModels {
		added[modelID] = struct{}{}
	}

	models := make([]objects.ChannelModelCapability, 0, len(previous.Models)+len(addedModels))
	existing := make(map[string]struct{}, len(previous.Models))
	for _, modelCapability := range previous.Models {
		if _, ok := removed[modelCapability.ModelID]; ok {
			continue
		}
		models = append(models, cloneChannelModelCapability(modelCapability))
		existing[modelCapability.ModelID] = struct{}{}
	}
	for _, modelID := range addedModels {
		if _, ok := existing[modelID]; ok {
			continue
		}
		if _, ok := added[modelID]; !ok {
			continue
		}
		models = append(models, objects.ChannelModelCapability{
			ModelID:   modelID,
			Protocols: append([]string(nil), previous.DeclaredProtocols...),
		})
	}

	return objects.ChannelProtocolCapabilities{
		DeclaredProtocols: append([]string(nil), previous.DeclaredProtocols...),
		Models:            models,
	}
}

func (svc *ChannelService) unmatchedCapabilityModels(ctx context.Context, models []objects.ChannelModelCapability) ([]string, error) {
	if len(models) == 0 {
		return []string{}, nil
	}
	modelIDs := lo.Map(models, func(model objects.ChannelModelCapability, _ int) string { return model.ModelID })
	found, err := svc.entFromContext(ctx).Model.Query().Where(model.ModelIDIn(modelIDs...)).Select(model.FieldModelID).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query capability model matches: %w", err)
	}
	foundSet := make(map[string]struct{}, len(found))
	for _, modelID := range found {
		foundSet[modelID] = struct{}{}
	}
	unmatched := make([]string, 0)
	for _, modelID := range modelIDs {
		if _, ok := foundSet[modelID]; !ok {
			unmatched = append(unmatched, modelID)
		}
	}

	return lo.Uniq(unmatched), nil
}

// reviewChannelEndpointCapabilities 按保存后的运行时端点复核已有能力声明。
func (svc *ChannelService) reviewChannelEndpointCapabilities(ctx context.Context, ch *ent.Channel) (*ent.Channel, *RevokedChannelCapabilitiesPayload, error) {
	if ch == nil {
		return ch, &RevokedChannelCapabilitiesPayload{ManualNotices: []string{}}, nil
	}
	previous := ch.ProtocolCapabilities
	current := filterCapabilitiesByEndpoints(ch, previous)
	if reflect.DeepEqual(previous, current) {
		return ch, &RevokedChannelCapabilitiesPayload{ManualNotices: []string{}}, nil
	}

	var updated *ent.Channel
	var stats AssociationReconcileStats
	var err error
	err = svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		updated, err = svc.entFromContext(txCtx).Channel.UpdateOneID(ch.ID).
			SetProtocolCapabilities(current).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("failed to revoke unsupported channel capabilities: %w", err)
		}
		stats, err = svc.reconcileChannelProtocolCapabilities(txCtx, ch.ID, previous, current)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	if ent.TxFromContext(ctx) == nil {
		updated.Unwrap()
	}

	return updated, &RevokedChannelCapabilitiesPayload{
		AutoDisabledCount: stats.AutoDisabledCount,
		ManualNotices:     stats.ManualNotices,
	}, nil
}

func filterCapabilitiesByEndpoints(ch *ent.Channel, capabilities objects.ChannelProtocolCapabilities) objects.ChannelProtocolCapabilities {
	declared := make([]string, 0, len(capabilities.DeclaredProtocols))
	declaredSet := make(map[string]struct{}, len(capabilities.DeclaredProtocols))
	for _, protocol := range capabilities.DeclaredProtocols {
		if !channelSupportsProtocolFamily(resolveChannelEndpoints(ch), protocol) {
			continue
		}
		declared = append(declared, protocol)
		declaredSet[protocol] = struct{}{}
	}
	models := make([]objects.ChannelModelCapability, 0, len(capabilities.Models))
	for _, modelCapability := range capabilities.Models {
		protocols := make([]string, 0, len(modelCapability.Protocols))
		for _, protocol := range modelCapability.Protocols {
			if _, ok := declaredSet[protocol]; ok {
				protocols = append(protocols, protocol)
			}
		}
		modelCapability.Protocols = protocols
		models = append(models, modelCapability)
	}

	return objects.ChannelProtocolCapabilities{DeclaredProtocols: declared, Models: models}
}
