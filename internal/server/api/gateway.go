package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"

	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

// GatewayHandlersParams Gateway 处理器依赖参数.
type GatewayHandlersParams struct {
	fx.In

	AdapterService *biz.AdapterService
}

// GatewayHandlers 负责适配器配置管理的 REST API 处理器.
type GatewayHandlers struct {
	AdapterService *biz.AdapterService
}

// NewGatewayHandlers 创建 Gateway 处理器.
func NewGatewayHandlers(params GatewayHandlersParams) *GatewayHandlers {
	return &GatewayHandlers{
		AdapterService: params.AdapterService,
	}
}

// ListAdaptersResponse 适配器列表响应.
type ListAdaptersResponse struct {
	Adapters []AdapterDTO `json:"adapters"`
}

// AdapterDTO 适配器数据传输对象.
type AdapterDTO struct {
	ID               int          `json:"id"`
	Name             string       `json:"name"`
	DisplayName      string       `json:"display_name"`
	InboundAPIFormat string       `json:"inbound_api_format"`
	Status           string       `json:"status"`
	Remark           *string      `json:"remark,omitempty"`
	Bindings         []BindingDTO `json:"bindings"`
}

// BindingDTO 适配器绑定数据传输对象.
type BindingDTO struct {
	ID            int     `json:"id"`
	SourceModelID string  `json:"source_model_id"`
	ModelGroupID  int     `json:"model_group_id"`
	Enabled       bool    `json:"enabled"`
	Remark        *string `json:"remark,omitempty"`
}

// ListAdapters 返回所有适配器配置（包含绑定信息）.
func (h *GatewayHandlers) ListAdapters(c *gin.Context) {
	adapters, err := h.AdapterService.ListAdapters(c.Request.Context())
	if err != nil {
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	response := ListAdaptersResponse{
		Adapters: make([]AdapterDTO, 0, len(adapters)),
	}

	for _, adapter := range adapters {
		bindings := make([]BindingDTO, 0, len(adapter.Bindings))
		for _, binding := range adapter.Bindings {
			bindings = append(bindings, BindingDTO{
				ID:            binding.ID,
				SourceModelID: binding.SourceModelID,
				ModelGroupID:  binding.ModelGroupID,
				Enabled:       binding.Enabled,
				Remark:        binding.Remark,
			})
		}

		response.Adapters = append(response.Adapters, AdapterDTO{
			ID:               adapter.ID,
			Name:             adapter.Name,
			DisplayName:      adapter.DisplayName,
			InboundAPIFormat: adapter.InboundAPIFormat,
			Status:           adapter.Status,
			Remark:           adapter.Remark,
			Bindings:         bindings,
		})
	}

	c.JSON(http.StatusOK, response)
}

// UpdateAdapterRequest 更新适配器请求.
type UpdateAdapterRequest struct {
	DisplayName      string  `json:"display_name"`
	InboundAPIFormat string  `json:"inbound_api_format"` // 创建时必填，更新时忽略
	Status           string  `json:"status"`
	Remark           *string `json:"remark,omitempty"`
	// Bindings 整体替换：传入完整的新绑定列表，不传则保留现有绑定
	Bindings []BindingInput `json:"bindings,omitempty"`
}

// BindingInput 绑定输入.
type BindingInput struct {
	SourceModelID string `json:"source_model_id" binding:"required"`
	ModelGroupID  int    `json:"model_group_id" binding:"required"`
	Enabled       bool   `json:"enabled"`
	Remark        string `json:"remark,omitempty"`
}

// UpdateAdapter 更新适配器配置，包含可选的整体绑定替换（事务操作）。
func (h *GatewayHandlers) UpdateAdapter(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		JSONError(c, http.StatusBadRequest, errors.New("adapter name is required"))
		return
	}

	var req UpdateAdapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err)
		return
	}

	// 验证 status 值
	if req.Status != "" && req.Status != "enabled" && req.Status != "disabled" && req.Status != "archived" {
		JSONError(c, http.StatusBadRequest, errors.New("invalid status: must be enabled, disabled, or archived"))
		return
	}

	// 验证 bindings 输入
	for _, binding := range req.Bindings {
		if binding.SourceModelID == "" {
			JSONError(c, http.StatusBadRequest, errors.New("binding source_model_id is required"))
			return
		}
		if binding.ModelGroupID <= 0 {
			JSONError(c, http.StatusBadRequest, errors.New("binding model_group_id must be positive"))
			return
		}
	}

	// 执行更新操作（包含事务）
	updatedAdapter, err := h.AdapterService.UpdateAdapter(c.Request.Context(), name, &biz.UpdateAdapterParams{
		DisplayName:      req.DisplayName,
		InboundAPIFormat: req.InboundAPIFormat,
		Status:           req.Status,
		Remark:           req.Remark,
		Bindings:         convertBindingInputs(req.Bindings),
	})
	if err != nil {
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	// 刷新快照
	result, err := h.AdapterService.Refresh(c.Request.Context())
	if err != nil {
		// 刷新失败，保留旧快照，返回错误
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"adapter":          updatedAdapter,
		"snapshot_version": result.SnapshotVersion,
		"refreshed_at":     result.RefreshedAt,
		"diagnostics":      result.Diagnostics,
	})
}

// ListModelGroupsResponse 模型组列表响应.
type ListModelGroupsResponse struct {
	ModelGroups []ModelGroupDTO `json:"model_groups"`
}

// ModelGroupDTO 模型组数据传输对象.
type ModelGroupDTO struct {
	ID                int           `json:"id"`
	Name              string        `json:"name"`
	DisplayName       string        `json:"display_name"`
	Status            string        `json:"status"`
	SelectionStrategy string        `json:"selection_strategy"`
	Remark            *string       `json:"remark,omitempty"`
	Protocols         []ProtocolDTO `json:"protocols"`
}

// ProtocolDTO 协议数据传输对象.
type ProtocolDTO struct {
	ID               int         `json:"id"`
	InboundAPIFormat string      `json:"inbound_api_format"`
	Enabled          bool        `json:"enabled"`
	Remark           *string     `json:"remark,omitempty"`
	Targets          []TargetDTO `json:"targets"`
}

// TargetDTO 目标数据传输对象.
type TargetDTO struct {
	ID                int                   `json:"id"`
	ChannelID         int                   `json:"channel_id"`
	TargetModelID     string                `json:"target_model_id"`
	OutboundAPIFormat string                `json:"outbound_api_format"`
	Priority          int                   `json:"priority"`
	Enabled           bool                  `json:"enabled"`
	Remark            *string               `json:"remark,omitempty"`
	Capabilities      TargetCapabilitiesDTO `json:"capabilities"`
}

// TargetCapabilitiesDTO 目标能力数据传输对象.
type TargetCapabilitiesDTO struct {
	SupportsTools    bool     `json:"supports_tools"`
	SupportsStream   bool     `json:"supports_stream"`
	// StreamPolicy 目标级流式响应策略："unlimited"、"require"、"forbid"。空字符串按旧数据兼容处理。
	StreamPolicy     string   `json:"stream_policy,omitempty"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

// ListModelGroups 返回所有模型组配置（包含协议和目标信息）.
func (h *GatewayHandlers) ListModelGroups(c *gin.Context) {
	modelGroups, err := h.AdapterService.ListModelGroups(c.Request.Context())
	if err != nil {
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	response := ListModelGroupsResponse{
		ModelGroups: make([]ModelGroupDTO, 0, len(modelGroups)),
	}

	for _, mg := range modelGroups {
		protocols := make([]ProtocolDTO, 0, len(mg.Protocols))
		for _, protocol := range mg.Protocols {
			targets := make([]TargetDTO, 0, len(protocol.Targets))
			for _, target := range protocol.Targets {
				targets = append(targets, TargetDTO{
					ID:                target.ID,
					ChannelID:         target.ChannelID,
					TargetModelID:     target.TargetModelID,
					OutboundAPIFormat: target.OutboundAPIFormat,
					Priority:          target.Priority,
					Enabled:           target.Enabled,
					Remark:            target.Remark,
					Capabilities: TargetCapabilitiesDTO{
						SupportsTools:    target.Capabilities.SupportsTools,
						SupportsStream:   target.Capabilities.SupportsStream,
						StreamPolicy:     target.Capabilities.StreamPolicy,
						InputModalities:  target.Capabilities.InputModalities,
						OutputModalities: target.Capabilities.OutputModalities,
					},
				})
			}

			protocols = append(protocols, ProtocolDTO{
				ID:               protocol.ID,
				InboundAPIFormat: protocol.InboundAPIFormat,
				Enabled:          protocol.Enabled,
				Remark:           protocol.Remark,
				Targets:          targets,
			})
		}

		response.ModelGroups = append(response.ModelGroups, ModelGroupDTO{
			ID:                mg.ID,
			Name:              mg.Name,
			DisplayName:       mg.DisplayName,
			Status:            mg.Status,
			SelectionStrategy: mg.SelectionStrategy,
			Remark:            mg.Remark,
			Protocols:         protocols,
		})
	}

	c.JSON(http.StatusOK, response)
}

// UpdateModelGroupRequest 更新模型组请求.
type UpdateModelGroupRequest struct {
	DisplayName       string  `json:"display_name"`
	Status            string  `json:"status"`
	SelectionStrategy string  `json:"selection_strategy"`
	Remark            *string `json:"remark,omitempty"`
	// Protocols 整体替换：传入完整的新协议列表（含目标），不传则保留现有配置
	Protocols []ProtocolInput `json:"protocols,omitempty"`
}

// ProtocolInput 协议输入.
type ProtocolInput struct {
	InboundAPIFormat string  `json:"inbound_api_format" binding:"required"`
	Enabled          bool    `json:"enabled"`
	Remark           *string `json:"remark,omitempty"`
	// Targets 整体替换
	Targets []TargetInput `json:"targets,omitempty"`
}

// TargetInput 目标输入.
type TargetInput struct {
	ChannelID         int                   `json:"channel_id" binding:"required"`
	TargetModelID     string                `json:"target_model_id" binding:"required"`
	OutboundAPIFormat string                `json:"outbound_api_format" binding:"required"`
	Priority          int                   `json:"priority"`
	Enabled           bool                  `json:"enabled"`
	Remark            *string               `json:"remark,omitempty"`
	Capabilities      TargetCapabilitiesDTO `json:"capabilities"`
}

// UpdateModelGroup 更新模型组配置，包含可选的整体协议/目标替换（事务操作）。
func (h *GatewayHandlers) UpdateModelGroup(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		JSONError(c, http.StatusBadRequest, errors.New("model group name is required"))
		return
	}

	var req UpdateModelGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err)
		return
	}

	// 验证 status 值
	if req.Status != "" && req.Status != "enabled" && req.Status != "disabled" && req.Status != "archived" {
		JSONError(c, http.StatusBadRequest, errors.New("invalid status: must be enabled, disabled, or archived"))
		return
	}

	// 验证 selection_strategy 值
	if req.SelectionStrategy != "" && req.SelectionStrategy != "priority_failover" {
		JSONError(c, http.StatusBadRequest, errors.New("invalid selection_strategy: must be priority_failover"))
		return
	}

	// 验证 protocols 输入
	for _, protocol := range req.Protocols {
		if protocol.InboundAPIFormat == "" {
			JSONError(c, http.StatusBadRequest, errors.New("protocol inbound_api_format is required"))
			return
		}
		for _, target := range protocol.Targets {
			if target.ChannelID <= 0 {
				JSONError(c, http.StatusBadRequest, errors.New("target channel_id must be positive"))
				return
			}
			if target.TargetModelID == "" {
				JSONError(c, http.StatusBadRequest, errors.New("target target_model_id is required"))
				return
			}
			if target.OutboundAPIFormat == "" {
				JSONError(c, http.StatusBadRequest, errors.New("target outbound_api_format is required"))
				return
			}
			// 校验 stream_policy 必须为合法枚举値
			sp := target.Capabilities.StreamPolicy
			if sp != "" && sp != "unlimited" && sp != "require" && sp != "forbid" {
				JSONError(c, http.StatusBadRequest, errors.New("invalid stream_policy: must be unlimited, require, or forbid"))
				return
			}
		}
	}

	// 执行更新操作（包含事务）
	updatedModelGroup, err := h.AdapterService.UpdateModelGroup(c.Request.Context(), name, &biz.UpdateModelGroupParams{
		DisplayName:       req.DisplayName,
		Status:            req.Status,
		SelectionStrategy: req.SelectionStrategy,
		Remark:            req.Remark,
		Protocols:         convertProtocolInputs(req.Protocols),
	})
	if err != nil {
		if errors.Is(err, biz.ErrModelGroupNotFound) {
			JSONError(c, http.StatusNotFound, errors.New("model group not found"))
			return
		}
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	// 刷新快照
	result, err := h.AdapterService.Refresh(c.Request.Context())
	if err != nil {
		// 刷新失败，保留旧快照，返回错误
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"model_group":      updatedModelGroup,
		"snapshot_version": result.SnapshotVersion,
		"refreshed_at":     result.RefreshedAt,
		"diagnostics":      result.Diagnostics,
	})
}

// GetRuntimeStatus 返回当前适配器运行时状态.
func (h *GatewayHandlers) GetRuntimeStatus(c *gin.Context) {
	status := h.AdapterService.RuntimeStatus()
	snapshot := h.AdapterService.Snapshot()

	c.JSON(http.StatusOK, gin.H{
		"status":                   status,
		"current_snapshot_version": snapshot.Version,
		"current_refreshed_at":     snapshot.RefreshedAt,
	})
}

// RefreshGateway 手动触发适配器配置刷新.
func (h *GatewayHandlers) RefreshGateway(c *gin.Context) {
	result, err := h.AdapterService.Refresh(c.Request.Context())
	if err != nil {
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshot_version": result.SnapshotVersion,
		"refreshed_at":     result.RefreshedAt,
		"diagnostics":      result.Diagnostics,
	})
}

// 辅助函数：转换 BindingInput 到 biz.BindingInput
func convertBindingInputs(inputs []BindingInput) []biz.BindingInput {
	if len(inputs) == 0 {
		return nil
	}
	result := make([]biz.BindingInput, 0, len(inputs))
	for _, input := range inputs {
		remark := input.Remark
		result = append(result, biz.BindingInput{
			SourceModelID: input.SourceModelID,
			ModelGroupID:  input.ModelGroupID,
			Enabled:       input.Enabled,
			Remark:        &remark,
		})
	}
	return result
}

// 辅助函数：转换 ProtocolInput 到 biz.ProtocolInput
func convertProtocolInputs(inputs []ProtocolInput) []biz.ProtocolInput {
	if len(inputs) == 0 {
		return nil
	}
	result := make([]biz.ProtocolInput, 0, len(inputs))
	for _, input := range inputs {
		protocolRemark := ""
		if input.Remark != nil {
			protocolRemark = *input.Remark
		}
		targets := make([]biz.TargetInput, 0, len(input.Targets))
		for _, target := range input.Targets {
			targetRemark := ""
			if target.Remark != nil {
				targetRemark = *target.Remark
			}
			targets = append(targets, biz.TargetInput{
				ChannelID:         target.ChannelID,
				TargetModelID:     target.TargetModelID,
				OutboundAPIFormat: target.OutboundAPIFormat,
				Priority:          target.Priority,
				Enabled:           target.Enabled,
				Remark:            &targetRemark,
				Capabilities: biz.AdapterTargetCapabilitiesInput{
					SupportsTools:    target.Capabilities.SupportsTools,
					SupportsStream:   target.Capabilities.SupportsStream,
					StreamPolicy:     target.Capabilities.StreamPolicy,
					InputModalities:  target.Capabilities.InputModalities,
					OutputModalities: target.Capabilities.OutputModalities,
				},
			})
		}
		result = append(result, biz.ProtocolInput{
			InboundAPIFormat: input.InboundAPIFormat,
			Enabled:          input.Enabled,
			Remark:           &protocolRemark,
			Targets:          targets,
		})
	}
	return result
}

// RenameAdapterRequest 适配器重命名请求.
type RenameAdapterRequest struct {
	NewName string `json:"new_name" binding:"required"`
}

// RenameAdapter 将适配器重命名为新名称。事务内完成：创建新适配器、迁移活跃绑定、软删除旧记录。
// 同名时返回当前配置（no-op）。
func (h *GatewayHandlers) RenameAdapter(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		JSONError(c, http.StatusBadRequest, errors.New("adapter name is required"))
		return
	}

	var req RenameAdapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err)
		return
	}
	if req.NewName == "" {
		JSONError(c, http.StatusBadRequest, errors.New("new_name is required"))
		return
	}

	result, err := h.AdapterService.RenameAdapter(c.Request.Context(), name, req.NewName)
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrAdapterNotFound):
			JSONError(c, http.StatusNotFound, err)
		case errors.Is(err, biz.ErrAdapterAlreadyExists):
			JSONError(c, http.StatusConflict, err)
		case errors.Is(err, biz.ErrAdapterInvalidName):
			JSONError(c, http.StatusBadRequest, err)
		default:
			JSONError(c, http.StatusInternalServerError, err)
		}
		return
	}

	// 刷新快照，使新名称立即生效
	refreshResult, err := h.AdapterService.Refresh(c.Request.Context())
	if err != nil {
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	// 构建返回 AdapterDTO
	bindings := make([]BindingDTO, 0, len(result.Bindings))
	for _, b := range result.Bindings {
		bindings = append(bindings, BindingDTO{
			ID:            b.ID,
			SourceModelID: b.SourceModelID,
			ModelGroupID:  b.ModelGroupID,
			Enabled:       b.Enabled,
			Remark:        b.Remark,
		})
	}
	adapterDTO := AdapterDTO{
		ID:               result.ID,
		Name:             result.Name,
		DisplayName:      result.DisplayName,
		InboundAPIFormat: result.InboundAPIFormat,
		Status:           result.Status,
		Remark:           result.Remark,
		Bindings:         bindings,
	}

	c.JSON(http.StatusOK, gin.H{
		"adapter":          adapterDTO,
		"snapshot_version": refreshResult.SnapshotVersion,
		"refreshed_at":     refreshResult.RefreshedAt,
		"diagnostics":      refreshResult.Diagnostics,
	})
}

// DeleteAdapter 软删除指定名称的适配器及其所有活跃绑定。
// 不存在时返回 404，已删除时也返回 404（幂等行为与现有项目习惯一致）。
func (h *GatewayHandlers) DeleteAdapter(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		JSONError(c, http.StatusBadRequest, errors.New("adapter name is required"))
		return
	}

	if err := h.AdapterService.DeleteAdapter(c.Request.Context(), name); err != nil {
		if errors.Is(err, biz.ErrAdapterNotFound) {
			JSONError(c, http.StatusNotFound, err)
		} else {
			JSONError(c, http.StatusInternalServerError, err)
		}
		return
	}

	// 刷新快照，使删除立即生效
	refreshResult, err := h.AdapterService.Refresh(c.Request.Context())
	if err != nil {
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshot_version": refreshResult.SnapshotVersion,
		"refreshed_at":     refreshResult.RefreshedAt,
		"diagnostics":      refreshResult.Diagnostics,
	})
}

// DeleteModelGroup 软删除指定名称的模型组（含协议和目标）。
// 若仍被活跃绑定引用，返回 409 并提示先移除绑定。不存在时返回 404。
func (h *GatewayHandlers) DeleteModelGroup(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		JSONError(c, http.StatusBadRequest, errors.New("model group name is required"))
		return
	}

	if err := h.AdapterService.DeleteModelGroup(c.Request.Context(), name); err != nil {
		switch {
		case errors.Is(err, biz.ErrModelGroupNotFound):
			JSONError(c, http.StatusNotFound, err)
		case errors.Is(err, biz.ErrModelGroupInUse):
			JSONError(c, http.StatusConflict, err)
		default:
			JSONError(c, http.StatusInternalServerError, err)
		}
		return
	}

	// 刷新快照，使删除立即生效
	refreshResult, err := h.AdapterService.Refresh(c.Request.Context())
	if err != nil {
		JSONError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshot_version": refreshResult.SnapshotVersion,
		"refreshed_at":     refreshResult.RefreshedAt,
		"diagnostics":      refreshResult.Diagnostics,
	})
}
