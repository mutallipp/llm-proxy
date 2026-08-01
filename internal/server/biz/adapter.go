package biz

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/fx"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/adapter"
	"github.com/mutallipp/llm-proxy/internal/ent/adaptermodelbinding"
	"github.com/mutallipp/llm-proxy/internal/ent/model"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

// RuntimeAdapter 等类型别名让业务层调用方不需要依赖 DTO 的具体存放位置。
type RuntimeAdapter = objects.RuntimeAdapter
type RuntimeAdapterBinding = objects.RuntimeAdapterBinding
type RuntimeModelGroup = objects.RuntimeModelGroup
type RuntimeModelGroupProtocol = objects.RuntimeModelGroupProtocol
type RuntimeModelGroupTarget = objects.RuntimeModelGroupTarget
type AdapterSnapshot = objects.AdapterSnapshot
type AdapterDiagnostic = objects.AdapterDiagnostic
type AdapterRuntimeStatus = objects.AdapterRuntimeStatus

// AdapterRefreshResult 描述一次刷新是否发布了新的完整快照。
type AdapterRefreshResult struct {
	SnapshotVersion uint64
	RefreshedAt     time.Time
	Diagnostics     []objects.AdapterDiagnostic
}

type AdapterServiceParams struct {
	fx.In

	Ent            *ent.Client
	ChannelService *ChannelService
}

// AdapterService 负责加载适配器配置并以不可变快照提供请求路径读取。
type AdapterService struct {
	*AbstractService

	ChannelService *ChannelService

	snapshot  atomic.Value // *objects.AdapterSnapshot
	refreshMu sync.Mutex
	statusMu  sync.RWMutex
	status    objects.AdapterRuntimeStatus
}

func NewAdapterService(params AdapterServiceParams) *AdapterService {
	initial := &objects.AdapterSnapshot{Adapters: map[string]*objects.RuntimeAdapter{}}

	svc := &AdapterService{
		AbstractService: &AbstractService{db: params.Ent},
		ChannelService:  params.ChannelService,
	}
	svc.snapshot.Store(initial)

	return svc
}

// Refresh 从数据库和 ChannelService 的启用渠道缓存构建完整快照。
// 所有校验完成后才发布，任何失败都会保留上一次成功快照。
func (svc *AdapterService) Refresh(ctx context.Context) (AdapterRefreshResult, error) {
	svc.refreshMu.Lock()
	defer svc.refreshMu.Unlock()

	oldSnapshot := svc.Snapshot()
	loaded, diagnostics, err := svc.loadSnapshot(ctx)
	if err != nil {
		svc.statusMu.Lock()
		svc.status.LastRefreshError = err.Error()
		svc.statusMu.Unlock()

		return AdapterRefreshResult{
			SnapshotVersion: oldSnapshot.Version,
			RefreshedAt:     oldSnapshot.RefreshedAt,
			Diagnostics:     oldSnapshot.Diagnostics,
		}, err
	}

	refreshedAt := time.Now()
	loaded.Version = oldSnapshot.Version + 1
	loaded.RefreshedAt = refreshedAt
	loaded.LastSuccessfulRefreshAt = refreshedAt
	loaded.Diagnostics = diagnostics

	// 原子存储只接收一个完整指针，避免请求路径看到半成品 map 或切片。
	svc.snapshot.Store(loaded)

	svc.statusMu.Lock()
	svc.status = objects.AdapterRuntimeStatus{
		SnapshotVersion:         loaded.Version,
		RefreshedAt:             refreshedAt,
		LastSuccessfulRefreshAt: refreshedAt,
	}
	svc.statusMu.Unlock()

	return AdapterRefreshResult{
		SnapshotVersion: loaded.Version,
		RefreshedAt:     refreshedAt,
		Diagnostics:     diagnostics,
	}, nil
}

// Snapshot 返回当前不可变快照。调用方不得修改返回值中的 map、切片或对象。
func (svc *AdapterService) Snapshot() *objects.AdapterSnapshot {
	return svc.snapshot.Load().(*objects.AdapterSnapshot)
}

// Resolve 只从当前快照解析适配器，不在请求路径访问数据库。
func (svc *AdapterService) Resolve(_ context.Context, adapterName string) (*objects.RuntimeAdapter, error) {
	adapterName = strings.TrimSpace(adapterName)
	if adapterName == "" {
		return nil, fmt.Errorf("%w: empty adapter name", ErrAdapterNotFound)
	}

	adapterConfig, ok := svc.Snapshot().Adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrAdapterNotFound, adapterName)
	}

	return adapterConfig, nil
}

// ListModels 返回适配器已启用绑定的逻辑模型 ID，顺序与绑定定义一致。
func (svc *AdapterService) ListModels(ctx context.Context, adapterName string) ([]string, error) {
	adapterConfig, err := svc.Resolve(ctx, adapterName)
	if err != nil {
		return nil, err
	}

	models := make([]string, 0, len(adapterConfig.BindingOrder))
	for _, sourceModelID := range adapterConfig.BindingOrder {
		binding := adapterConfig.Bindings[sourceModelID]
		if binding == nil || !binding.Enabled {
			continue
		}

		models = append(models, sourceModelID)
	}

	return models, nil
}

// RuntimeStatus 返回最近一次刷新状态。刷新失败时当前快照版本保持不变。
func (svc *AdapterService) RuntimeStatus() objects.AdapterRuntimeStatus {
	svc.statusMu.RLock()
	defer svc.statusMu.RUnlock()

	return svc.status
}

func (svc *AdapterService) loadSnapshot(ctx context.Context) (*objects.AdapterSnapshot, []objects.AdapterDiagnostic, error) {
	db := svc.entFromContext(ctx)
	adapters, err := db.Adapter.Query().Where(adapter.StatusEQ(adapter.StatusEnabled)).Order(ent.Asc(adapter.FieldID)).All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load enabled adapters: %w", err)
	}
	bindings, err := db.AdapterModelBinding.Query().Where(adaptermodelbinding.EnabledEQ(true)).WithModel().Order(ent.Asc(adaptermodelbinding.FieldID)).All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load enabled adapter bindings: %w", err)
	}
	runtime := make(map[string]*objects.RuntimeAdapter, len(adapters))
	byID := make(map[int]*objects.RuntimeAdapter)
	for _, a := range adapters {
		if strings.TrimSpace(a.Name) == "" {
			return nil, nil, fmt.Errorf("enabled adapter %d has empty name", a.ID)
		}
		if a.InboundAPIFormat == "" {
			return nil, nil, fmt.Errorf("adapter %q has empty inbound api format", a.Name)
		}
		if _, ok := runtime[a.Name]; ok {
			return nil, nil, fmt.Errorf("duplicate enabled adapter name %q", a.Name)
		}
		r := &objects.RuntimeAdapter{ID: a.ID, Name: a.Name, DisplayName: a.DisplayName, InboundAPIFormat: a.InboundAPIFormat, Bindings: map[string]*objects.RuntimeAdapterBinding{}}
		runtime[a.Name] = r
		byID[a.ID] = r
	}
	for _, b := range bindings {
		r := byID[b.AdapterID]
		if r == nil {
			continue
		}
		m := b.Edges.Model
		if m == nil {
			return nil, nil, fmt.Errorf("adapter %q binding %d model does not exist", r.Name, b.ID)
		}
		if m.Status != model.StatusEnabled || m.DeletedAt != 0 {
			return nil, nil, fmt.Errorf("adapter %q binding model %q is disabled or deleted", r.Name, b.SourceModelID)
		}
		if m.Settings == nil {
			return nil, nil, fmt.Errorf("adapter %q model %q has no settings", r.Name, b.SourceModelID)
		}
		if _, ok := r.Bindings[b.SourceModelID]; ok {
			return nil, nil, fmt.Errorf("duplicate binding source model %q", b.SourceModelID)
		}
		r.Bindings[b.SourceModelID] = &objects.RuntimeAdapterBinding{ID: b.ID, SourceModelID: b.SourceModelID, Enabled: b.Enabled, Model: &objects.RuntimeModel{ID: m.ID, ModelID: m.ModelID, Name: m.Name, Settings: m.Settings}}
		r.BindingOrder = append(r.BindingOrder, b.SourceModelID)
	}
	return &objects.AdapterSnapshot{Adapters: runtime}, nil, nil
}
func hasEndpoint(endpoints []objects.ChannelEndpoint, apiFormat string) bool {
	for _, endpoint := range endpoints {
		if endpoint.APIFormat == apiFormat {
			return true
		}
	}

	return false
}

func cloneTargetCapabilities(capabilities objects.AdapterTargetCapabilities) objects.AdapterTargetCapabilities {
	return objects.AdapterTargetCapabilities{
		SupportsTools:     capabilities.SupportsTools,
		SupportsStream:    capabilities.SupportsStream,
		StreamPolicy:      capabilities.StreamPolicy,
		SupportsReasoning: capabilities.SupportsReasoning,
		ContextLength:     capabilities.ContextLength,
		MaxOutputTokens:   capabilities.MaxOutputTokens,
		InputModalities:   append([]string(nil), capabilities.InputModalities...),
		OutputModalities:  append([]string(nil), capabilities.OutputModalities...),
	}
}

func cloneRuntimeTargets(targets []*objects.RuntimeModelGroupTarget) []*objects.RuntimeModelGroupTarget {
	cloned := make([]*objects.RuntimeModelGroupTarget, 0, len(targets))
	for _, target := range targets {
		if target == nil {
			continue
		}

		copyTarget := *target
		copyTarget.Capabilities = cloneTargetCapabilities(target.Capabilities)
		cloned = append(cloned, &copyTarget)
	}

	return cloned
}

// --- 适配器配置管理 API 的业务方法 ---

// AdapterInfo 适配器信息（含绑定）
type AdapterInfo struct {
	ID               int
	Name             string
	DisplayName      string
	InboundAPIFormat string
	Status           string
	Remark           *string
	Bindings         []BindingInfo
}

// BindingInfo 绑定信息
type BindingInfo struct {
	ID            int
	SourceModelID string
	ModelID       int
	Enabled       bool
	Remark        *string
}

// ModelGroupInfo 模型组信息（含协议和目标）
type ModelGroupInfo struct {
	ID                int
	Name              string
	DisplayName       string
	Status            string
	SelectionStrategy string
	Remark            *string
	Protocols         []ProtocolInfo
}

// ProtocolInfo 协议信息
type ProtocolInfo struct {
	ID               int
	InboundAPIFormat string
	Enabled          bool
	Remark           *string
	Targets          []TargetInfo
}

// TargetInfo 目标信息
type TargetInfo struct {
	ID                int
	ChannelID         int
	TargetModelID     string
	OutboundAPIFormat string
	Priority          int
	Enabled           bool
	Remark            *string
	Capabilities      objects.AdapterTargetCapabilities
}

// UpdateAdapterParams 更新适配器参数
type UpdateAdapterParams struct {
	DisplayName      string
	InboundAPIFormat string // 创建时必填，更新时忽略
	Status           string
	Remark           *string
	Bindings         []BindingInput
}

// BindingInput 绑定输入
type BindingInput struct {
	SourceModelID string
	ModelID       int
	Enabled       bool
	Remark        *string
}

// UpdateModelGroupParams 更新模型组参数
type UpdateModelGroupParams struct {
	DisplayName       string
	Status            string
	SelectionStrategy string
	Remark            *string
	Protocols         []ProtocolInput
}

// ProtocolInput 协议输入
type ProtocolInput struct {
	InboundAPIFormat string
	Enabled          bool
	Remark           *string
	Targets          []TargetInput
}

// TargetInput 目标输入
type TargetInput struct {
	ChannelID         int
	TargetModelID     string
	OutboundAPIFormat string
	Priority          int
	Enabled           bool
	Remark            *string
	Capabilities      AdapterTargetCapabilitiesInput
}

// AdapterTargetCapabilitiesInput 目标能力输入
type AdapterTargetCapabilitiesInput struct {
	SupportsTools  bool
	SupportsStream bool
	// StreamPolicy 目标级流式策略："unlimited" | "require" | "forbid" | ""
	StreamPolicy      string
	SupportsReasoning bool
	ContextLength     int
	MaxOutputTokens   int
	InputModalities   []string
	OutputModalities  []string
}

var (
	ErrModelGroupNotFound = fmt.Errorf("model group not found")
	ErrProtocolNotFound   = fmt.Errorf("protocol not found")
)

// ListAdapters 返回所有适配器及其绑定信息（软删除的不返回）
func (svc *AdapterService) ListAdapters(ctx context.Context) ([]AdapterInfo, error) {
	db := svc.entFromContext(ctx)

	adapters, err := db.Adapter.Query().
		Where(adapter.DeletedAtEQ(0)).
		Order(ent.Asc(adapter.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query adapters: %w", err)
	}

	result := make([]AdapterInfo, 0, len(adapters))
	for _, a := range adapters {
		bindings, err := db.AdapterModelBinding.Query().
			Where(adaptermodelbinding.AdapterID(a.ID)).
			Order(ent.Asc(adaptermodelbinding.FieldID)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query bindings for adapter %d: %w", a.ID, err)
		}

		bindingInfos := make([]BindingInfo, 0, len(bindings))
		for _, b := range bindings {
			bindingInfos = append(bindingInfos, BindingInfo{
				ID:            b.ID,
				SourceModelID: b.SourceModelID,
				ModelID:       b.ModelID,
				Enabled:       b.Enabled,
				Remark:        b.Remark,
			})
		}

		result = append(result, AdapterInfo{
			ID:               a.ID,
			Name:             a.Name,
			DisplayName:      a.DisplayName,
			InboundAPIFormat: a.InboundAPIFormat,
			Status:           a.Status.String(),
			Remark:           a.Remark,
			Bindings:         bindingInfos,
		})
	}

	return result, nil
}

// UpdateAdapter 更新适配器配置，包含可选的整体绑定替换（事务操作）
func (svc *AdapterService) UpdateAdapter(ctx context.Context, name string, params *UpdateAdapterParams) (*AdapterInfo, error) {
	if params == nil {
		return nil, fmt.Errorf("params is required")
	}

	var result *AdapterInfo
	err := svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		db := svc.entFromContext(txCtx)

		// 查找适配器，不存在则创建（upsert）
		a, err := db.Adapter.Query().
			Where(adapter.Name(name), adapter.DeletedAtEQ(0)).
			Only(txCtx)
		if err != nil {
			if !ent.IsNotFound(err) {
				return fmt.Errorf("failed to query adapter: %w", err)
			}
			// 创建新适配器
			if params.InboundAPIFormat == "" {
				return fmt.Errorf("inbound_api_format is required when creating adapter %q", name)
			}
			create := db.Adapter.Create().
				SetName(name).
				SetInboundAPIFormat(params.InboundAPIFormat)
			if params.DisplayName != "" {
				create = create.SetDisplayName(params.DisplayName)
			}
			if params.Status != "" {
				create = create.SetStatus(adapter.Status(params.Status))
			}
			if params.Remark != nil {
				create = create.SetRemark(*params.Remark)
			}
			a, err = create.Save(txCtx)
			if err != nil {
				return fmt.Errorf("failed to create adapter: %w", err)
			}
		} else {
			// 更新适配器字段
			update := db.Adapter.UpdateOne(a)
			if params.DisplayName != "" {
				update = update.SetDisplayName(params.DisplayName)
			}
			if params.Status != "" {
				update = update.SetStatus(adapter.Status(params.Status))
			}
			if params.Remark != nil {
				update = update.SetRemark(*params.Remark)
			}
			_, err = update.Save(txCtx)
			if err != nil {
				return fmt.Errorf("failed to update adapter: %w", err)
			}
		}

		// 如果传入了 bindings，整体替换
		if len(params.Bindings) > 0 {
			// 软删除现有绑定
			existingBindings, err := db.AdapterModelBinding.Query().
				Where(adaptermodelbinding.AdapterID(a.ID)).
				All(txCtx)
			if err != nil {
				return fmt.Errorf("failed to query existing bindings: %w", err)
			}
			for _, eb := range existingBindings {
				_, err := db.AdapterModelBinding.UpdateOne(eb).SetDeletedAt(int(time.Now().Unix())).Save(txCtx)
				if err != nil {
					return fmt.Errorf("failed to soft delete binding: %w", err)
				}
			}

			// 创建新绑定
			for _, binding := range params.Bindings {
				remark := ""
				if binding.Remark != nil {
					remark = *binding.Remark
				}
				_, err := db.AdapterModelBinding.Create().
					SetAdapterID(a.ID).
					SetSourceModelID(binding.SourceModelID).
					SetModelID(binding.ModelID).
					SetEnabled(binding.Enabled).
					SetRemark(remark).
					Save(txCtx)
				if err != nil {
					return fmt.Errorf("failed to create binding: %w", err)
				}
			}
		}

		// 查询更新后的适配器信息
		a, err = db.Adapter.Query().
			Where(adapter.ID(a.ID)).
			Only(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query updated adapter: %w", err)
		}

		bindings, err := db.AdapterModelBinding.Query().
			Where(adaptermodelbinding.AdapterID(a.ID)).
			Order(ent.Asc(adaptermodelbinding.FieldID)).
			All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query bindings: %w", err)
		}

		bindingInfos := make([]BindingInfo, 0, len(bindings))
		for _, b := range bindings {
			bindingInfos = append(bindingInfos, BindingInfo{
				ID:            b.ID,
				SourceModelID: b.SourceModelID,
				ModelID:       b.ModelID,
				Enabled:       b.Enabled,
				Remark:        b.Remark,
			})
		}

		result = &AdapterInfo{
			ID:               a.ID,
			Name:             a.Name,
			DisplayName:      a.DisplayName,
			InboundAPIFormat: a.InboundAPIFormat,
			Status:           a.Status.String(),
			Remark:           a.Remark,
			Bindings:         bindingInfos,
		}

		return nil
	})

	return result, err
}

// ListModelGroups 返回所有模型组及其协议和目标信息
func (svc *AdapterService) ListModelGroups(ctx context.Context) ([]ModelGroupInfo, error) {
	db := svc.entFromContext(ctx)

	groups, err := db.ModelGroup.Query().
		Where(modelgroup.DeletedAtEQ(0)).
		Order(ent.Asc(modelgroup.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query model groups: %w", err)
	}

	result := make([]ModelGroupInfo, 0, len(groups))
	for _, g := range groups {
		protocols, err := db.ModelGroupProtocol.Query().
			Where(modelgroupprotocol.ModelGroupID(g.ID)).
			Order(ent.Asc(modelgroupprotocol.FieldID)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query protocols for model group %d: %w", g.ID, err)
		}

		protocolInfos := make([]ProtocolInfo, 0, len(protocols))
		for _, p := range protocols {
			targets, err := db.ModelGroupTarget.Query().
				Where(modelgrouptarget.ModelGroupProtocolID(p.ID)).
				Order(ent.Asc(modelgrouptarget.FieldPriority), ent.Asc(modelgrouptarget.FieldID)).
				All(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to query targets for protocol %d: %w", p.ID, err)
			}

			targetInfos := make([]TargetInfo, 0, len(targets))
			for _, t := range targets {
				targetInfos = append(targetInfos, TargetInfo{
					ID:                t.ID,
					ChannelID:         t.ChannelID,
					TargetModelID:     t.TargetModelID,
					OutboundAPIFormat: t.OutboundAPIFormat,
					Priority:          t.Priority,
					Enabled:           t.Enabled,
					Remark:            t.Remark,
					Capabilities:      t.Capabilities,
				})
			}

			protocolInfos = append(protocolInfos, ProtocolInfo{
				ID:               p.ID,
				InboundAPIFormat: p.InboundAPIFormat,
				Enabled:          p.Enabled,
				Remark:           p.Remark,
				Targets:          targetInfos,
			})
		}

		result = append(result, ModelGroupInfo{
			ID:                g.ID,
			Name:              g.Name,
			DisplayName:       g.DisplayName,
			Status:            g.Status.String(),
			SelectionStrategy: g.SelectionStrategy.String(),
			Remark:            g.Remark,
			Protocols:         protocolInfos,
		})
	}

	return result, nil
}

// UpdateModelGroup 更新模型组配置，包含可选的整体协议/目标替换（事务操作）
func (svc *AdapterService) UpdateModelGroup(ctx context.Context, name string, params *UpdateModelGroupParams) (*ModelGroupInfo, error) {
	if params == nil {
		return nil, fmt.Errorf("params is required")
	}

	var result *ModelGroupInfo
	err := svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		db := svc.entFromContext(txCtx)

		// 查找模型组，不存在则创建（upsert）
		g, err := db.ModelGroup.Query().
			Where(modelgroup.Name(name), modelgroup.DeletedAtEQ(0)).
			Only(txCtx)
		if err != nil {
			if !ent.IsNotFound(err) {
				return fmt.Errorf("failed to query model group: %w", err)
			}
			// 创建新模型组
			create := db.ModelGroup.Create().SetName(name)
			if params.DisplayName != "" {
				create = create.SetDisplayName(params.DisplayName)
			}
			if params.Status != "" {
				create = create.SetStatus(modelgroup.Status(params.Status))
			}
			if params.SelectionStrategy != "" {
				create = create.SetSelectionStrategy(modelgroup.SelectionStrategy(params.SelectionStrategy))
			}
			if params.Remark != nil {
				create = create.SetRemark(*params.Remark)
			}
			g, err = create.Save(txCtx)
			if err != nil {
				return fmt.Errorf("failed to create model group: %w", err)
			}
		} else {
			// 更新模型组字段
			update := db.ModelGroup.UpdateOne(g)
			if params.DisplayName != "" {
				update = update.SetDisplayName(params.DisplayName)
			}
			if params.Status != "" {
				update = update.SetStatus(modelgroup.Status(params.Status))
			}
			if params.SelectionStrategy != "" {
				update = update.SetSelectionStrategy(modelgroup.SelectionStrategy(params.SelectionStrategy))
			}
			if params.Remark != nil {
				update = update.SetRemark(*params.Remark)
			}
			_, err = update.Save(txCtx)
			if err != nil {
				return fmt.Errorf("failed to update model group: %w", err)
			}
		}

		// 如果传入了 protocols，整体替换
		if len(params.Protocols) > 0 {
			// 软删除现有协议
			existingProtocols, err := db.ModelGroupProtocol.Query().
				Where(modelgroupprotocol.ModelGroupID(g.ID)).
				All(txCtx)
			if err != nil {
				return fmt.Errorf("failed to query existing protocols: %w", err)
			}
			for _, ep := range existingProtocols {
				// 软删除协议下的所有目标
				existingTargets, err := db.ModelGroupTarget.Query().
					Where(modelgrouptarget.ModelGroupProtocolID(ep.ID)).
					All(txCtx)
				if err != nil {
					return fmt.Errorf("failed to query targets for protocol: %w", err)
				}
				for _, et := range existingTargets {
					_, err := db.ModelGroupTarget.UpdateOne(et).SetDeletedAt(int(time.Now().Unix())).Save(txCtx)
					if err != nil {
						return fmt.Errorf("failed to soft delete target: %w", err)
					}
				}
				_, err = db.ModelGroupProtocol.UpdateOne(ep).SetDeletedAt(int(time.Now().Unix())).Save(txCtx)
				if err != nil {
					return fmt.Errorf("failed to soft delete protocol: %w", err)
				}
			}

			// 创建新协议和目标
			for _, protocol := range params.Protocols {
				remark := ""
				if protocol.Remark != nil {
					remark = *protocol.Remark
				}
				p, err := db.ModelGroupProtocol.Create().
					SetModelGroupID(g.ID).
					SetInboundAPIFormat(protocol.InboundAPIFormat).
					SetEnabled(protocol.Enabled).
					SetRemark(remark).
					Save(txCtx)
				if err != nil {
					return fmt.Errorf("failed to create protocol: %w", err)
				}

				// 创建目标
				for _, target := range protocol.Targets {
					targetRemark := ""
					if target.Remark != nil {
						targetRemark = *target.Remark
					}
					capabilities := objects.AdapterTargetCapabilities{
						SupportsTools:     target.Capabilities.SupportsTools,
						SupportsStream:    target.Capabilities.SupportsStream,
						StreamPolicy:      target.Capabilities.StreamPolicy,
						SupportsReasoning: target.Capabilities.SupportsReasoning,
						ContextLength:     target.Capabilities.ContextLength,
						MaxOutputTokens:   target.Capabilities.MaxOutputTokens,
						InputModalities:   target.Capabilities.InputModalities,
						OutputModalities:  target.Capabilities.OutputModalities,
					}
					_, err := db.ModelGroupTarget.Create().
						SetModelGroupProtocolID(p.ID).
						SetChannelID(target.ChannelID).
						SetTargetModelID(target.TargetModelID).
						SetOutboundAPIFormat(target.OutboundAPIFormat).
						SetPriority(target.Priority).
						SetEnabled(target.Enabled).
						SetRemark(targetRemark).
						SetCapabilities(capabilities).
						Save(txCtx)
					if err != nil {
						return fmt.Errorf("failed to create target: %w", err)
					}
				}
			}
		}

		// 查询更新后的模型组信息
		g, err = db.ModelGroup.Query().
			Where(modelgroup.ID(g.ID)).
			Only(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query updated model group: %w", err)
		}

		protocols, err := db.ModelGroupProtocol.Query().
			Where(modelgroupprotocol.ModelGroupID(g.ID)).
			Order(ent.Asc(modelgroupprotocol.FieldID)).
			All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query protocols: %w", err)
		}

		protocolInfos := make([]ProtocolInfo, 0, len(protocols))
		for _, p := range protocols {
			targets, err := db.ModelGroupTarget.Query().
				Where(modelgrouptarget.ModelGroupProtocolID(p.ID)).
				Order(ent.Asc(modelgrouptarget.FieldPriority), ent.Asc(modelgrouptarget.FieldID)).
				All(txCtx)
			if err != nil {
				return fmt.Errorf("failed to query targets: %w", err)
			}

			targetInfos := make([]TargetInfo, 0, len(targets))
			for _, t := range targets {
				targetInfos = append(targetInfos, TargetInfo{
					ID:                t.ID,
					ChannelID:         t.ChannelID,
					TargetModelID:     t.TargetModelID,
					OutboundAPIFormat: t.OutboundAPIFormat,
					Priority:          t.Priority,
					Enabled:           t.Enabled,
					Remark:            t.Remark,
					Capabilities:      t.Capabilities,
				})
			}

			protocolInfos = append(protocolInfos, ProtocolInfo{
				ID:               p.ID,
				InboundAPIFormat: p.InboundAPIFormat,
				Enabled:          p.Enabled,
				Remark:           p.Remark,
				Targets:          targetInfos,
			})
		}

		result = &ModelGroupInfo{
			ID:                g.ID,
			Name:              g.Name,
			DisplayName:       g.DisplayName,
			Status:            g.Status.String(),
			SelectionStrategy: g.SelectionStrategy.String(),
			Remark:            g.Remark,
			Protocols:         protocolInfos,
		}

		return nil
	})

	return result, err
}

// adapterNamePattern 合法的 Adapter 名称：首字符字母或数字，后续可含连字符和下划线。
// 和消费侧路由中 /:adapter/v1 对应，名称不能含斜杠或空白。
var adapterNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ValidateAdapterName 校验 Adapter 名称是否合法，返回 ErrAdapterInvalidName 或 nil。
func ValidateAdapterName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrAdapterInvalidName)
	}
	if !adapterNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q — must start with a letter or digit and contain only letters, digits, hyphens, and underscores", ErrAdapterInvalidName, name)
	}
	return nil
}

// RenameAdapterParams 重命名适配器参数。
type RenameAdapterParams struct {
	NewName string
}

// RenameAdapter 安全重命名适配器：事务内创建新适配器并迁移活跃绑定，软删除旧记录。
// 若新旧名称相同则返回当前配置（no-op）。
func (svc *AdapterService) RenameAdapter(ctx context.Context, oldName, newName string) (*AdapterInfo, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)

	// 校验新名称格式
	if err := ValidateAdapterName(newName); err != nil {
		return nil, err
	}

	// 同名 no-op：直接返回当前配置
	if oldName == newName {
		db := svc.entFromContext(ctx)
		a, err := db.Adapter.Query().
			Where(adapter.Name(oldName), adapter.DeletedAtEQ(0)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, fmt.Errorf("%w: %q", ErrAdapterNotFound, oldName)
			}
			return nil, fmt.Errorf("failed to query adapter: %w", err)
		}
		return svc.buildAdapterInfo(ctx, a)
	}

	var result *AdapterInfo
	err := svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		db := svc.entFromContext(txCtx)

		// 查找旧适配器（不存在则返回 404）
		oldAdapter, err := db.Adapter.Query().
			Where(adapter.Name(oldName), adapter.DeletedAtEQ(0)).
			Only(txCtx)
		if err != nil {
			if ent.IsNotFound(err) {
				return fmt.Errorf("%w: %q", ErrAdapterNotFound, oldName)
			}
			return fmt.Errorf("failed to query adapter: %w", err)
		}

		// 检查新名称未被其他未删除适配器占用
		conflict, err := db.Adapter.Query().
			Where(adapter.Name(newName), adapter.DeletedAtEQ(0)).
			Exist(txCtx)
		if err != nil {
			return fmt.Errorf("failed to check name conflict: %w", err)
		}
		if conflict {
			return fmt.Errorf("%w: %q", ErrAdapterAlreadyExists, newName)
		}

		// 创建新适配器，保留全部元信息（inboundAPIFormat、displayName、status、remark）
		createOp := db.Adapter.Create().
			SetName(newName).
			SetInboundAPIFormat(oldAdapter.InboundAPIFormat).
			SetDisplayName(oldAdapter.DisplayName).
			SetStatus(oldAdapter.Status)
		if oldAdapter.Remark != nil {
			createOp = createOp.SetRemark(*oldAdapter.Remark)
		}
		newAdapter, err := createOp.Save(txCtx)
		if err != nil {
			return fmt.Errorf("failed to create renamed adapter: %w", err)
		}

		// 加载旧适配器的活跃绑定（deleted_at == 0）
		activeBindings, err := db.AdapterModelBinding.Query().
			Where(adaptermodelbinding.AdapterID(oldAdapter.ID), adaptermodelbinding.DeletedAtEQ(0)).
			Order(ent.Asc(adaptermodelbinding.FieldID)).
			All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query active bindings: %w", err)
		}

		// 为新适配器创建对应绑定
		for _, b := range activeBindings {
			createBinding := db.AdapterModelBinding.Create().
				SetAdapterID(newAdapter.ID).
				SetModelID(b.ModelID).
				SetSourceModelID(b.SourceModelID).
				SetEnabled(b.Enabled)
			if b.Remark != nil {
				createBinding = createBinding.SetRemark(*b.Remark)
			}
			if _, err := createBinding.Save(txCtx); err != nil {
				return fmt.Errorf("failed to migrate binding %d: %w", b.ID, err)
			}
		}

		// 软删除旧绑定（包含全部，不仅仅是活跃的，保持一致性）
		now := int(time.Now().Unix())
		allOldBindings, err := db.AdapterModelBinding.Query().
			Where(adaptermodelbinding.AdapterID(oldAdapter.ID), adaptermodelbinding.DeletedAtEQ(0)).
			All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query old bindings: %w", err)
		}
		for _, b := range allOldBindings {
			if _, err := db.AdapterModelBinding.UpdateOne(b).SetDeletedAt(now).Save(txCtx); err != nil {
				return fmt.Errorf("failed to soft-delete old binding %d: %w", b.ID, err)
			}
		}

		// 软删除旧适配器
		if _, err := db.Adapter.UpdateOne(oldAdapter).SetDeletedAt(now).Save(txCtx); err != nil {
			return fmt.Errorf("failed to soft-delete old adapter: %w", err)
		}

		// 构建返回值
		bindings, err := db.AdapterModelBinding.Query().
			Where(adaptermodelbinding.AdapterID(newAdapter.ID), adaptermodelbinding.DeletedAtEQ(0)).
			Order(ent.Asc(adaptermodelbinding.FieldID)).
			All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query new bindings: %w", err)
		}

		bindingInfos := make([]BindingInfo, 0, len(bindings))
		for _, b := range bindings {
			bindingInfos = append(bindingInfos, BindingInfo{
				ID:            b.ID,
				SourceModelID: b.SourceModelID,
				ModelID:       b.ModelID,
				Enabled:       b.Enabled,
				Remark:        b.Remark,
			})
		}
		result = &AdapterInfo{
			ID:               newAdapter.ID,
			Name:             newAdapter.Name,
			DisplayName:      newAdapter.DisplayName,
			InboundAPIFormat: newAdapter.InboundAPIFormat,
			Status:           newAdapter.Status.String(),
			Remark:           newAdapter.Remark,
			Bindings:         bindingInfos,
		}
		return nil
	})
	return result, err
}

// DeleteAdapter 安全软删除适配器及其所有活跃绑定。
// 适配器不存在时返回 ErrAdapterNotFound。
func (svc *AdapterService) DeleteAdapter(ctx context.Context, name string) error {
	return svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		db := svc.entFromContext(txCtx)

		// 查找适配器（不存在则 404）
		a, err := db.Adapter.Query().
			Where(adapter.Name(name), adapter.DeletedAtEQ(0)).
			Only(txCtx)
		if err != nil {
			if ent.IsNotFound(err) {
				return fmt.Errorf("%w: %q", ErrAdapterNotFound, name)
			}
			return fmt.Errorf("failed to query adapter: %w", err)
		}

		now := int(time.Now().Unix())

		// 软删除所有活跃绑定
		activeBindings, err := db.AdapterModelBinding.Query().
			Where(adaptermodelbinding.AdapterID(a.ID), adaptermodelbinding.DeletedAtEQ(0)).
			All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query active bindings: %w", err)
		}
		for _, b := range activeBindings {
			if _, err := db.AdapterModelBinding.UpdateOne(b).SetDeletedAt(now).Save(txCtx); err != nil {
				return fmt.Errorf("failed to soft-delete binding %d: %w", b.ID, err)
			}
		}

		// 软删除适配器
		if _, err := db.Adapter.UpdateOne(a).SetDeletedAt(now).Save(txCtx); err != nil {
			return fmt.Errorf("failed to soft-delete adapter: %w", err)
		}

		return nil
	})
}

// DeleteModelGroup 安全软删除模型组（含协议和目标）。
// 若仍有活跃 AdapterModelBinding 引用该组，则返回 ErrModelGroupInUse（409）。
// 模型组不存在时返回 ErrModelGroupNotFound（404）。
func (svc *AdapterService) DeleteModelGroup(ctx context.Context, name string) error {
	return svc.RunInTransaction(ctx, func(txCtx context.Context) error {
		db := svc.entFromContext(txCtx)

		// 查找模型组（不存在则 404）
		g, err := db.ModelGroup.Query().
			Where(modelgroup.Name(name), modelgroup.DeletedAtEQ(0)).
			Only(txCtx)
		if err != nil {
			if ent.IsNotFound(err) {
				return fmt.Errorf("%w: %q", ErrModelGroupNotFound, name)
			}
			return fmt.Errorf("failed to query model group: %w", err)
		}

		// 检查是否被活跃绑定引用（引用 → 409）
		refCount, err := db.AdapterModelBinding.Query().
			Where(adaptermodelbinding.ModelID(g.ID), adaptermodelbinding.DeletedAtEQ(0)).
			Count(txCtx)
		if err != nil {
			return fmt.Errorf("failed to check active bindings: %w", err)
		}
		if refCount > 0 {
			return fmt.Errorf("%w: %d active binding(s) still reference model group %q — remove them first", ErrModelGroupInUse, refCount, name)
		}

		now := int(time.Now().Unix())

		// 加载该组的所有协议（含已软删除的，避免遗漏；仅对活跃协议做级联）
		protocols, err := db.ModelGroupProtocol.Query().
			Where(modelgroupprotocol.ModelGroupID(g.ID), modelgroupprotocol.DeletedAtEQ(0)).
			All(txCtx)
		if err != nil {
			return fmt.Errorf("failed to query protocols: %w", err)
		}
		for _, p := range protocols {
			// 软删除协议下的所有活跃目标
			targets, err := db.ModelGroupTarget.Query().
				Where(modelgrouptarget.ModelGroupProtocolID(p.ID), modelgrouptarget.DeletedAtEQ(0)).
				All(txCtx)
			if err != nil {
				return fmt.Errorf("failed to query targets for protocol %d: %w", p.ID, err)
			}
			for _, t := range targets {
				if _, err := db.ModelGroupTarget.UpdateOne(t).SetDeletedAt(now).Save(txCtx); err != nil {
					return fmt.Errorf("failed to soft-delete target %d: %w", t.ID, err)
				}
			}
			// 软删除协议
			if _, err := db.ModelGroupProtocol.UpdateOne(p).SetDeletedAt(now).Save(txCtx); err != nil {
				return fmt.Errorf("failed to soft-delete protocol %d: %w", p.ID, err)
			}
		}

		// 软删除模型组
		if _, err := db.ModelGroup.UpdateOne(g).SetDeletedAt(now).Save(txCtx); err != nil {
			return fmt.Errorf("failed to soft-delete model group: %w", err)
		}

		return nil
	})
}

// buildAdapterInfo 从 ent.Adapter 构建 AdapterInfo（含活跃绑定）。
func (svc *AdapterService) buildAdapterInfo(ctx context.Context, a *ent.Adapter) (*AdapterInfo, error) {
	db := svc.entFromContext(ctx)
	bindings, err := db.AdapterModelBinding.Query().
		Where(adaptermodelbinding.AdapterID(a.ID), adaptermodelbinding.DeletedAtEQ(0)).
		Order(ent.Asc(adaptermodelbinding.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query bindings: %w", err)
	}
	infos := make([]BindingInfo, 0, len(bindings))
	for _, b := range bindings {
		infos = append(infos, BindingInfo{
			ID:            b.ID,
			SourceModelID: b.SourceModelID,
			ModelID:       b.ModelID,
			Enabled:       b.Enabled,
			Remark:        b.Remark,
		})
	}
	return &AdapterInfo{
		ID:               a.ID,
		Name:             a.Name,
		DisplayName:      a.DisplayName,
		InboundAPIFormat: a.InboundAPIFormat,
		Status:           a.Status.String(),
		Remark:           a.Remark,
		Bindings:         infos,
	}, nil
}
