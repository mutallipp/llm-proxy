package biz

import (
	"context"
	"fmt"
	"reflect"

	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

// ModelCapabilityValidationError 是派生关联启用失败时返回的结构化错误。
type ModelCapabilityValidationError struct {
	ChannelID int
	ModelID   string
	Protocol  string
	Reason    string
}

func (e *ModelCapabilityValidationError) Error() string {
	return fmt.Sprintf("cannot enable derived association for channel %d, model %q, protocol %q: %s", e.ChannelID, e.ModelID, e.Protocol, e.Reason)
}

func (svc *ModelService) prepareNewModelSettings(ctx context.Context, settings *objects.ModelSettings) (*objects.ModelSettings, error) {
	prepared := cloneModelSettings(settings)
	for protocol, associations := range prepared.ProtocolPools {
		for _, association := range associations {
			if association == nil {
				continue
			}
			// 新模型没有服务端存量状态；原因字段始终由服务端清空。
			association.DisabledReason = ""
			if association.Auto && !association.Disabled {
				if err := svc.validateDerivedAssociationEnablement(ctx, protocol, association); err != nil {
					return nil, err
				}
			}
		}
	}

	return prepared, nil
}

func (svc *ModelService) prepareUpdatedModelSettings(ctx context.Context, settings, existing *objects.ModelSettings) (*objects.ModelSettings, error) {
	prepared := cloneModelSettings(settings)
	existingAuto := make(map[modelAssociationKey]*objects.ModelAssociation)
	if existing != nil {
		for protocol, associations := range existing.ProtocolPools {
			for _, association := range associations {
				if association == nil || !association.Auto {
					continue
				}
				if key, ok := modelAssociationIdentity(protocol, association); ok {
					existingAuto[key] = association
				}
			}
		}
	}

	seenAuto := make(map[modelAssociationKey]struct{}, len(existingAuto))
	for protocol, associations := range prepared.ProtocolPools {
		for _, association := range associations {
			if association == nil {
				continue
			}
			key, hasIdentity := modelAssociationIdentity(protocol, association)
			existingAssociation, wasAuto := existingAuto[key]
			if wasAuto && hasIdentity {
				seenAuto[key] = struct{}{}
				if modelAssociationDefinitionChanged(existingAssociation, association) {
					// 编辑自动条目即转为手动条目，但保留用户提交的禁用状态。
					association.Auto = false
					association.DisabledReason = ""
					continue
				}
				association.Auto = true
				association.DisabledReason = existingAssociation.DisabledReason
				if !association.Disabled {
					if err := svc.validateDerivedAssociationEnablement(ctx, protocol, association); err != nil {
						return nil, err
					}
					association.Auto = false
					association.DisabledReason = ""
				}
				continue
			}
			// 新增关联以及客户端伪造的 auto 标记均按手动条目处理。
			association.Auto = false
			association.DisabledReason = ""
		}
	}

	for key := range existingAuto {
		if _, ok := seenAuto[key]; !ok {
			return nil, fmt.Errorf("cannot delete existing derived association for channel %d, model %q, protocol %q", key.ChannelID, key.ModelID, key.Protocol)
		}
	}

	return prepared, nil
}

type modelAssociationKey struct {
	Protocol  string
	ChannelID int
	ModelID   string
}

func modelAssociationIdentity(protocol string, association *objects.ModelAssociation) (modelAssociationKey, bool) {
	if association == nil || association.Type != "channel_model" || association.ChannelModel == nil {
		return modelAssociationKey{}, false
	}

	return modelAssociationKey{
		Protocol:  protocol,
		ChannelID: association.ChannelModel.ChannelID,
		ModelID:   association.ChannelModel.ModelID,
	}, true
}

func modelAssociationDefinitionChanged(existing, incoming *objects.ModelAssociation) bool {
	left := cloneModelAssociation(existing)
	right := cloneModelAssociation(incoming)
	for _, association := range []*objects.ModelAssociation{left, right} {
		if association == nil {
			continue
		}
		association.Priority = 0
		association.Disabled = false
		association.Auto = false
		association.DisabledReason = ""
	}

	return !reflect.DeepEqual(left, right)
}

// validateDerivedAssociationEnablement 校验 auto 派生条目启用所需的四项能力。
func (svc *ModelService) validateDerivedAssociationEnablement(ctx context.Context, protocol string, association *objects.ModelAssociation) error {
	if association == nil || association.ChannelModel == nil {
		return fmt.Errorf("derived association must target a channel model")
	}
	channelID := association.ChannelModel.ChannelID
	modelID := association.ChannelModel.ModelID
	ch, err := svc.entFromContext(ctx).Channel.Get(ctx, channelID)
	if err != nil {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道不存在"}
	}
	if ch.Status != channel.StatusEnabled {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道未启用"}
	}
	if !channelCapabilityDeclaresProtocol(ch.ProtocolCapabilities, modelID, protocol) {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道当前未声明该模型与协议"}
	}
	if !channelSupportsProtocolFamily(ch, protocol) {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道端点不支持该协议"}
	}
	entries := (&Channel{Channel: ch}).GetModelEntries()
	if _, ok := entries[modelID]; !ok {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "物理模型不在渠道运行时模型条目中"}
	}

	return nil
}

func channelCapabilityDeclaresProtocol(capabilities objects.ChannelProtocolCapabilities, modelID, protocol string) bool {
	for _, modelCapability := range capabilities.Models {
		if modelCapability.ModelID != modelID {
			continue
		}
		for _, declaredProtocol := range modelCapability.Protocols {
			if declaredProtocol == protocol {
				return true
			}
		}
	}

	return false
}

// BulkEnableDerivedAssociationsInput 指定需要批量启用的渠道派生关联。
type BulkEnableDerivedAssociationsInput struct {
	ChannelID objects.GUID
}

// BulkEnableDerivedAssociationResult 是单条派生关联的启用结果。
type BulkEnableDerivedAssociationResult struct {
	AssociationID string
	Success       bool
	Reason        *string
}

// BulkEnableDerivedAssociationsResult 是批量启用的逐条结果。
type BulkEnableDerivedAssociationsResult struct {
	Results []*BulkEnableDerivedAssociationResult
}

// DeriveModelAssociationsPayload 是一键同步渠道后的统计结果。
type DeriveModelAssociationsPayload struct {
	AddedCount int
}

// BulkEnableDerivedAssociations 对指定渠道的自动派生条目逐条执行启用校验。
func (svc *ModelService) BulkEnableDerivedAssociations(ctx context.Context, input BulkEnableDerivedAssociationsInput) (*BulkEnableDerivedAssociationsResult, error) {
	channelID := input.ChannelID.ID
	var results []*BulkEnableDerivedAssociationResult
	err := svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		models, err := svc.entFromContext(txCtx).Model.Query().All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query models for bulk derived association enablement: %w", err)
		}
		for _, logicalModel := range models {
			if logicalModel.Settings == nil {
				continue
			}
			settings := cloneModelSettings(logicalModel.Settings)
			changed := false
			for protocol, associations := range settings.ProtocolPools {
				for _, association := range associations {
					if association == nil || !isDerivedChannelModelAssociation(association, channelID) {
						continue
					}
					associationID := fmt.Sprintf("%s:%d:%s", protocol, channelID, association.ChannelModel.ModelID)
					if !association.Disabled {
						if err := validateDerivedAssociationEnablementWithChannelService(txCtx, svc.channelService, protocol, association); err != nil {
							reason := err.Error()
							results = append(results, &BulkEnableDerivedAssociationResult{AssociationID: associationID, Success: false, Reason: &reason})
							continue
						}
						association.Auto = false
						association.DisabledReason = ""
						changed = true
						results = append(results, &BulkEnableDerivedAssociationResult{AssociationID: associationID, Success: true})
						continue
					}
					if err := validateDerivedAssociationEnablementWithChannelService(txCtx, svc.channelService, protocol, association); err != nil {
						reason := err.Error()
						results = append(results, &BulkEnableDerivedAssociationResult{AssociationID: associationID, Success: false, Reason: &reason})
						continue
					}
					association.Disabled = false
					association.Auto = false
					association.DisabledReason = ""
					changed = true
					results = append(results, &BulkEnableDerivedAssociationResult{AssociationID: associationID, Success: true})
				}
			}
			if changed {
				if _, err := svc.entFromContext(txCtx).Model.UpdateOneID(logicalModel.ID).SetSettings(settings).Save(txCtx); err != nil {
					return fmt.Errorf("failed to save bulk-enabled derived associations for model %d: %w", logicalModel.ID, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &BulkEnableDerivedAssociationsResult{Results: results}, nil
}

// DeriveModelAssociations 按逻辑模型名查询本地能力表并返回新增统计。
func (svc *ModelService) DeriveModelAssociations(ctx context.Context, modelID int) (*DeriveModelAssociationsPayload, error) {
	_, stats, err := svc.channelService.DeriveModelAssociations(ctx, modelID)
	if err != nil {
		return nil, err
	}

	return &DeriveModelAssociationsPayload{AddedCount: stats.AddedCount}, nil
}

func validateDerivedAssociationEnablementWithChannelService(ctx context.Context, channelService *ChannelService, protocol string, association *objects.ModelAssociation) error {
	if association == nil || association.ChannelModel == nil {
		return fmt.Errorf("derived association must target a channel model")
	}
	channelID := association.ChannelModel.ChannelID
	modelID := association.ChannelModel.ModelID
	ch, err := channelService.entFromContext(ctx).Channel.Get(ctx, channelID)
	if err != nil {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道不存在"}
	}
	if ch.Status != channel.StatusEnabled {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道未启用"}
	}
	if !channelCapabilityDeclaresProtocol(ch.ProtocolCapabilities, modelID, protocol) {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道当前未声明该模型与协议"}
	}
	if !channelSupportsProtocolFamily(ch, protocol) {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "渠道端点不支持该协议"}
	}
	entries := (&Channel{Channel: ch}).GetModelEntries()
	if _, ok := entries[modelID]; !ok {
		return &ModelCapabilityValidationError{ChannelID: channelID, ModelID: modelID, Protocol: protocol, Reason: "物理模型不在渠道运行时模型条目中"}
	}

	return nil
}
