package biz

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

const (
	// ProtocolPoolDisabledReasonCapabilityRevoked 是能力声明撤销时的自动禁用原因。
	ProtocolPoolDisabledReasonCapabilityRevoked = "渠道协议声明已撤销"
	// ProtocolPoolDisabledReasonChannelDeleted 是渠道删除时手动关联的禁用原因。
	ProtocolPoolDisabledReasonChannelDeleted = "渠道已删除"
)

// AssociationReconcileStats 描述一次协议池对账的结果。
type AssociationReconcileStats struct {
	AddedCount        int
	AutoDisabledCount int
	AutoRestoredCount int
	ManualNotices     []string
}

// reconcileAssociations 根据渠道的新能力声明对单个模型的本地协议池做纯函数对账。
// effectivePools 用于判重，localPools 是唯一允许被写回的模型级 settings。
func reconcileAssociations(
	channelID int,
	previous objects.ChannelProtocolCapabilities,
	current objects.ChannelProtocolCapabilities,
	localPools map[string][]*objects.ModelAssociation,
	effectivePools map[string][]*objects.ModelAssociation,
) (map[string][]*objects.ModelAssociation, AssociationReconcileStats) {
	return reconcileAssociationsWithMode(channelID, previous, current, localPools, effectivePools, true)
}

// reconcileAssociationsIncremental 只新增或恢复派生关联，不因当前声明缺失而撤销。
func reconcileAssociationsIncremental(
	channelID int,
	current objects.ChannelProtocolCapabilities,
	localPools map[string][]*objects.ModelAssociation,
	effectivePools map[string][]*objects.ModelAssociation,
) (map[string][]*objects.ModelAssociation, AssociationReconcileStats) {
	return reconcileAssociationsWithMode(channelID, objects.ChannelProtocolCapabilities{}, current, localPools, effectivePools, false)
}

func reconcileAssociationsWithMode(
	channelID int,
	previous objects.ChannelProtocolCapabilities,
	current objects.ChannelProtocolCapabilities,
	localPools map[string][]*objects.ModelAssociation,
	effectivePools map[string][]*objects.ModelAssociation,
	revokeMissing bool,
) (map[string][]*objects.ModelAssociation, AssociationReconcileStats) {
	result := cloneProtocolPools(localPools)
	stats := AssociationReconcileStats{}
	declared := capabilityAssociationSet(current)
	previouslyDeclared := capabilityAssociationSet(previous)

	// 先处理现有自动条目，保证撤销与恢复都保留原有优先级和实体。
	for protocol, associations := range result {
		for index, association := range associations {
			if !isDerivedChannelModelAssociation(association, channelID) {
				continue
			}

			key := protocolModelKey{Protocol: protocol, ModelID: association.ChannelModel.ModelID}
			if !revokeMissing {
				if _, ok := declared[key]; ok && association.Disabled && association.DisabledReason == ProtocolPoolDisabledReasonCapabilityRevoked {
					association.Disabled = true
					association.DisabledReason = ""
					stats.AutoRestoredCount++
				}
				associations[index] = association
				continue
			}

			if _, ok := declared[key]; ok {
				if association.Disabled && association.DisabledReason == ProtocolPoolDisabledReasonCapabilityRevoked {
					association.DisabledReason = ""
					stats.AutoRestoredCount++
				}
				associations[index] = association
				continue
			}

			if !association.Disabled || association.DisabledReason != ProtocolPoolDisabledReasonCapabilityRevoked {
				association.Disabled = true
				association.DisabledReason = ProtocolPoolDisabledReasonCapabilityRevoked
				stats.AutoDisabledCount++
			}
			associations[index] = association
		}
		result[protocol] = associations
	}

	// 再为声明中尚不存在的模型×协议创建自动条目。
	for key := range declared {
		if hasAssociationForChannel(effectivePools[key.Protocol], channelID, key.ModelID) {
			continue
		}
		if hasAssociationForChannel(result[key.Protocol], channelID, key.ModelID) {
			continue
		}

		result[key.Protocol] = append(result[key.Protocol], &objects.ModelAssociation{
			Type:     "channel_model",
			Priority: 0,
			Disabled: true,
			Auto:     true,
			ChannelModel: &objects.ChannelModelAssociation{
				ChannelID: channelID,
				ModelID:   key.ModelID,
			},
		})
		stats.AddedCount++
	}

	// 只有全量对账才报告被撤销声明背书的手动条目；手动条目本身不修改。
	if revokeMissing {
		for key := range previouslyDeclared {
			if _, ok := declared[key]; ok {
				continue
			}
			for _, association := range effectivePools[key.Protocol] {
				if association == nil || association.Auto || !isDerivedChannelModelAssociation(association, channelID) || association.ChannelModel.ModelID != key.ModelID {
					continue
				}
				stats.ManualNotices = append(stats.ManualNotices, fmt.Sprintf("%s/%s", key.Protocol, key.ModelID))
			}
		}
		sort.Strings(stats.ManualNotices)
	}

	return result, stats
}

type protocolModelKey struct {
	Protocol string
	ModelID  string
}

func capabilityAssociationSet(capabilities objects.ChannelProtocolCapabilities) map[protocolModelKey]struct{} {
	declared := make(map[protocolModelKey]struct{})
	for _, modelCapability := range capabilities.Models {
		for _, protocol := range modelCapability.Protocols {
			declared[protocolModelKey{Protocol: protocol, ModelID: modelCapability.ModelID}] = struct{}{}
		}
	}

	return declared
}

func isDerivedChannelModelAssociation(association *objects.ModelAssociation, channelID int) bool {
	return association != nil && association.Auto && association.Type == "channel_model" && association.ChannelModel != nil && association.ChannelModel.ChannelID == channelID
}

func hasAssociationForChannel(associations []*objects.ModelAssociation, channelID int, modelID string) bool {
	for _, association := range associations {
		if association == nil || association.Type != "channel_model" || association.ChannelModel == nil || association.ChannelModel.ChannelID != channelID || association.ChannelModel.ModelID != modelID {
			continue
		}
		return true
	}

	return false
}

func cloneProtocolPools(pools map[string][]*objects.ModelAssociation) map[string][]*objects.ModelAssociation {
	if pools == nil {
		return map[string][]*objects.ModelAssociation{}
	}

	result := make(map[string][]*objects.ModelAssociation, len(pools))
	for protocol, associations := range pools {
		result[protocol] = make([]*objects.ModelAssociation, len(associations))
		for index, association := range associations {
			result[protocol][index] = cloneModelAssociation(association)
		}
	}

	return result
}

func protocolPoolsEqual(left, right map[string][]*objects.ModelAssociation) bool {
	return reflect.DeepEqual(left, right)
}

// reconcileChannelProtocolCapabilities 在单个事务内对所有逻辑模型执行能力声明对账。
func (svc *ChannelService) reconcileChannelProtocolCapabilities(
	ctx context.Context,
	channelID int,
	previous objects.ChannelProtocolCapabilities,
	current objects.ChannelProtocolCapabilities,
) (AssociationReconcileStats, error) {
	var stats AssociationReconcileStats

	err := svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		models, err := svc.entFromContext(txCtx).Model.Query().All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query models for protocol pool derivation: %w", err)
		}
		systemSettings, err := svc.SystemService.ModelSettings(txCtx)
		if err != nil {
			return err
		}

		for _, logicalModel := range models {
			localPools := map[string][]*objects.ModelAssociation{}
			if logicalModel.Settings != nil {
				localPools = logicalModel.Settings.ProtocolPools
			}
			effectivePools := EffectiveModelProtocolPools(systemSettings, logicalModel)
			mergedPools, modelStats := reconcileAssociations(channelID, previous, current, localPools, effectivePools)
			if protocolPoolsEqual(localPools, mergedPools) {
				continue
			}

			settings := cloneModelSettings(logicalModel.Settings)
			settings.ProtocolPools = mergedPools
			if _, err := svc.entFromContext(txCtx).Model.UpdateOneID(logicalModel.ID).SetSettings(settings).Save(txCtx); err != nil {
				return fmt.Errorf("failed to write derived protocol pools for model %d: %w", logicalModel.ID, err)
			}
			stats.AddedCount += modelStats.AddedCount
			stats.AutoDisabledCount += modelStats.AutoDisabledCount
			stats.AutoRestoredCount += modelStats.AutoRestoredCount
			stats.ManualNotices = append(stats.ManualNotices, modelStats.ManualNotices...)
		}

		return nil
	})
	if err != nil {
		return AssociationReconcileStats{}, err
	}
	sort.Strings(stats.ManualNotices)

	return stats, nil
}

func cloneModelSettings(settings *objects.ModelSettings) *objects.ModelSettings {
	if settings == nil {
		return &objects.ModelSettings{ProtocolPools: map[string][]*objects.ModelAssociation{}}
	}

	clone := *settings
	clone.ProtocolPools = cloneProtocolPools(settings.ProtocolPools)

	return &clone
}

// deriveModelAssociationsForModel 按逻辑模型名反查本地能力表，只做增量新增/恢复。
func (svc *ChannelService) deriveModelAssociationsForModel(ctx context.Context, logicalModelID int) (*ent.Model, AssociationReconcileStats, error) {
	var updated *ent.Model
	var stats AssociationReconcileStats

	err := svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		logicalModel, err := svc.entFromContext(txCtx).Model.Get(txCtx, logicalModelID)
		if err != nil {
			return fmt.Errorf("failed to get logical model: %w", err)
		}
		channels, err := svc.entFromContext(txCtx).Channel.Query().All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query channels for protocol pool derivation: %w", err)
		}
		systemSettings, err := svc.SystemService.ModelSettings(txCtx)
		if err != nil {
			return err
		}

		localPools := map[string][]*objects.ModelAssociation{}
		if logicalModel.Settings != nil {
			localPools = logicalModel.Settings.ProtocolPools
		}
		mergedPools := cloneProtocolPools(localPools)
		for _, ch := range channels {
			current := ch.ProtocolCapabilities
			if !capabilityDeclaresModel(current, logicalModel.ModelID) {
				continue
			}
			effective := EffectiveModelProtocolPools(systemSettings, logicalModel)
			var modelStats AssociationReconcileStats
			mergedPools, modelStats = reconcileAssociationsIncremental(ch.ID, current, mergedPools, effective)
			stats.AddedCount += modelStats.AddedCount
			stats.AutoRestoredCount += modelStats.AutoRestoredCount
		}

		if !protocolPoolsEqual(localPools, mergedPools) {
			settings := cloneModelSettings(logicalModel.Settings)
			settings.ProtocolPools = mergedPools
			updated, err = svc.entFromContext(txCtx).Model.UpdateOneID(logicalModel.ID).SetSettings(settings).Save(txCtx)
			if err != nil {
				return fmt.Errorf("failed to write derived protocol pools for model %d: %w", logicalModel.ID, err)
			}
		} else {
			updated = logicalModel
		}

		return nil
	})
	if err != nil {
		return nil, AssociationReconcileStats{}, err
	}
	if ent.TxFromContext(ctx) == nil && updated != nil {
		updated.Unwrap()
	}

	return updated, stats, nil
}

func capabilityDeclaresModel(capabilities objects.ChannelProtocolCapabilities, modelID string) bool {
	for _, modelCapability := range capabilities.Models {
		if modelCapability.ModelID == modelID && len(modelCapability.Protocols) > 0 {
			return true
		}
	}

	return false
}

// cleanupDeletedChannelAssociations 清理渠道删除后的模型级关联。
func (svc *ChannelService) cleanupDeletedChannelAssociations(ctx context.Context, channelIDs []int) error {
	if len(channelIDs) == 0 {
		return nil
	}
	channelSet := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		channelSet[channelID] = struct{}{}
	}

	return svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		models, err := svc.entFromContext(txCtx).Model.Query().All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query models for deleted channel cleanup: %w", err)
		}
		for _, logicalModel := range models {
			if logicalModel.Settings == nil {
				continue
			}
			settings := cloneModelSettings(logicalModel.Settings)
			changed := false
			for protocol, associations := range settings.ProtocolPools {
				kept := make([]*objects.ModelAssociation, 0, len(associations))
				for _, association := range associations {
					if association == nil || association.Type != "channel_model" || association.ChannelModel == nil {
						kept = append(kept, association)
						continue
					}
					if _, ok := channelSet[association.ChannelModel.ChannelID]; !ok {
						kept = append(kept, association)
						continue
					}
					if association.Auto {
						changed = true
						continue
					}
					if !association.Disabled || association.DisabledReason != ProtocolPoolDisabledReasonChannelDeleted {
						association.Disabled = true
						association.DisabledReason = ProtocolPoolDisabledReasonChannelDeleted
						changed = true
					}
					kept = append(kept, association)
				}
				settings.ProtocolPools[protocol] = kept
			}
			if changed {
				if _, err := svc.entFromContext(txCtx).Model.UpdateOneID(logicalModel.ID).SetSettings(settings).Save(txCtx); err != nil {
					return fmt.Errorf("failed to clean deleted channel associations for model %d: %w", logicalModel.ID, err)
				}
			}
		}

		return nil
	})
}

// CleanupDeletedChannelAssociations 暴露渠道删除清理，供单条和批量删除共用。
func (svc *ChannelService) CleanupDeletedChannelAssociations(ctx context.Context, channelIDs []int) error {
	return svc.cleanupDeletedChannelAssociations(ctx, channelIDs)
}

// ReconcileChannelProtocolCapabilities 暴露能力对账，供能力服务和同步钩子调用。
func (svc *ChannelService) ReconcileChannelProtocolCapabilities(ctx context.Context, channelID int, previous, current objects.ChannelProtocolCapabilities) (AssociationReconcileStats, error) {
	return svc.reconcileChannelProtocolCapabilities(ctx, channelID, previous, current)
}

// DeriveModelAssociations 按模型名增量派生渠道协议池关联。
func (svc *ChannelService) DeriveModelAssociations(ctx context.Context, logicalModelID int) (*ent.Model, AssociationReconcileStats, error) {
	return svc.deriveModelAssociationsForModel(ctx, logicalModelID)
}
