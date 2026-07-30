package biz

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/adapter"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/modelgroup"
	"github.com/looplj/axonhub/internal/objects"
)

// setupTestAdapterService 创建用于测试的 AdapterService 和 ChannelService。
func setupTestAdapterService(t *testing.T) (*AdapterService, *ChannelService, *ent.Client) {
	t.Helper()

	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	channelSvc := NewChannelServiceForTest(client)

	adapterSvc := NewAdapterService(AdapterServiceParams{
		Ent:            client,
		ChannelService: channelSvc,
	})

	return adapterSvc, channelSvc, client
}

// createAdapterTestChannel 创建测试渠道并返回其 ID。
func createAdapterTestChannel(t *testing.T, client *ent.Client, ctx context.Context, channelType channel.Type, name, baseURL string, supportedModels []string, status channel.Status) int {
	t.Helper()

	ch, err := client.Channel.Create().
		SetType(channelType).
		SetName(name).
		SetBaseURL(baseURL).
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels(supportedModels).
		SetDefaultTestModel(supportedModels[0]).
		SetStatus(status).
		Save(ctx)
	require.NoError(t, err)

	return ch.ID
}

// newTestBizChannel 创建用于测试的 biz.Channel。
func newTestBizChannel(id int, channelType channel.Type, name, baseURL string, supportedModels []string, endpoints []objects.ChannelEndpoint) *Channel {
	return &Channel{
		Channel: &ent.Channel{
			ID:               id,
			Type:             channelType,
			Name:             name,
			BaseURL:          baseURL,
			SupportedModels:  supportedModels,
			DefaultTestModel: supportedModels[0],
			Endpoints:        endpoints,
		},
	}
}

// createTestAdapter 创建测试适配器。
func createTestAdapter(t *testing.T, client *ent.Client, name, inboundAPIFormat string, status adapter.Status) *ent.Adapter {
	t.Helper()

	a, err := client.Adapter.Create().
		SetName(name).
		SetInboundAPIFormat(inboundAPIFormat).
		SetStatus(status).
		Save(context.Background())
	require.NoError(t, err)

	return a
}

// createTestModelGroup 创建测试模型组。
func createTestModelGroup(t *testing.T, client *ent.Client, name string, status modelgroup.Status) *ent.ModelGroup {
	t.Helper()

	g, err := client.ModelGroup.Create().
		SetName(name).
		SetStatus(status).
		Save(context.Background())
	require.NoError(t, err)

	return g
}

// createTestProtocol 创建测试协议。
func createTestProtocol(t *testing.T, client *ent.Client, groupID int, inboundAPIFormat string) *ent.ModelGroupProtocol {
	t.Helper()

	p, err := client.ModelGroupProtocol.Create().
		SetModelGroupID(groupID).
		SetInboundAPIFormat(inboundAPIFormat).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)

	return p
}

// createTestTarget 创建测试目标。
func createTestTarget(t *testing.T, client *ent.Client, protocolID, channelID int, targetModelID, outboundAPIFormat string, priority int) *ent.ModelGroupTarget {
	t.Helper()

	tgt, err := client.ModelGroupTarget.Create().
		SetModelGroupProtocolID(protocolID).
		SetChannelID(channelID).
		SetTargetModelID(targetModelID).
		SetOutboundAPIFormat(outboundAPIFormat).
		SetPriority(priority).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)

	return tgt
}

// createTestBinding 创建测试绑定。
func createTestBinding(t *testing.T, client *ent.Client, adapterID, modelGroupID int, sourceModelID string, enabled bool) *ent.AdapterModelBinding {
	t.Helper()

	b, err := client.AdapterModelBinding.Create().
		SetAdapterID(adapterID).
		SetModelGroupID(modelGroupID).
		SetSourceModelID(sourceModelID).
		SetEnabled(enabled).
		Save(context.Background())
	require.NoError(t, err)

	return b
}

// createAdapterEndpoint 创建渠道端点。
func createAdapterEndpoint(apiFormat, basePath string) objects.ChannelEndpoint {
	return objects.ChannelEndpoint{
		APIFormat: apiFormat,
		Path:      basePath,
	}
}

// setupAdapterTestData 创建完整的适配器测试数据。
func setupAdapterTestData(t *testing.T, client *ent.Client, ctx context.Context, channelSvc *ChannelService) (
	*AdapterService, // adapterSvc
	int, // channelID
	*ent.Adapter, // adapter
	*ent.ModelGroup, // modelGroup
) {
	t.Helper()

	adapterSvc, _, _ := setupTestAdapterService(t)

	// 创建渠道
	chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Test Channel", "https://api.openai.com/v1", []string{"gpt-4", "gpt-3.5-turbo"}, channel.StatusEnabled)
	channelSvc.SetEnabledChannelsForTest([]*Channel{
		newTestBizChannel(chID, channel.TypeOpenai, "Test Channel", "https://api.openai.com/v1", []string{"gpt-4", "gpt-3.5-turbo"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
	})

	// 创建适配器
	adapterEnt := createTestAdapter(t, client, "test-adapter", "openai/chat/completions", adapter.StatusEnabled)

	// 创建模型组
	modelGroup := createTestModelGroup(t, client, "test-group", modelgroup.StatusEnabled)

	// 创建协议
	protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

	// 创建目标
	createTestTarget(t, client, protocol.ID, chID, "gpt-4", "openai/chat/completions", 1)

	// 创建绑定
	createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

	return adapterSvc, chID, adapterEnt, modelGroup
}

// TestAdapterService_Resolve 测试 Resolve 方法。
func TestAdapterService_Resolve(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("resolve existing adapter", func(t *testing.T) {
		// 刷新快照以加载数据
		_, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 创建适配器
		adapterEnt := createTestAdapter(t, client, "my-adapter", "openai/chat/completions", adapter.StatusEnabled)

		// 再次刷新
		_, err = svc.Refresh(ctx)
		require.NoError(t, err)

		// 解析
		result, err := svc.Resolve(ctx, "my-adapter")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "my-adapter", result.Name)
		require.Equal(t, adapterEnt.ID, result.ID)
		require.Equal(t, "openai/chat/completions", result.InboundAPIFormat)
	})

	t.Run("resolve non-existent adapter", func(t *testing.T) {
		_, err := svc.Resolve(ctx, "non-existent")
		require.Error(t, err)
		require.Contains(t, err.Error(), "adapter not found")
	})

	t.Run("resolve with empty name", func(t *testing.T) {
		_, err := svc.Resolve(ctx, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty adapter name")
	})

	t.Run("resolve with whitespace", func(t *testing.T) {
		// 先刷新快照
		_, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 测试空白字符会被 trim
		_, err = svc.Resolve(ctx, "  ")
		require.Error(t, err)
	})

	t.Run("resolve disabled adapter", func(t *testing.T) {
		// 创建已禁用的适配器
		createTestAdapter(t, client, "disabled-adapter", "openai/chat/completions", adapter.StatusDisabled)

		// 刷新
		_, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 解析应该失败，因为适配器未启用
		_, err = svc.Resolve(ctx, "disabled-adapter")
		require.Error(t, err)
	})
}

// TestAdapterService_ListModels 测试 ListModels 方法。
func TestAdapterService_ListModels(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 设置渠道
	chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Test Channel", "https://api.openai.com/v1", []string{"gpt-4", "gpt-3.5-turbo"}, channel.StatusEnabled)
	channelSvc.SetEnabledChannelsForTest([]*Channel{
		newTestBizChannel(chID, channel.TypeOpenai, "Test Channel", "https://api.openai.com/v1", []string{"gpt-4", "gpt-3.5-turbo"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
	})

	// 创建适配器
	adapterEnt := createTestAdapter(t, client, "list-models-adapter", "openai/chat/completions", adapter.StatusEnabled)

	// 创建模型组
	modelGroup := createTestModelGroup(t, client, "list-models-group", modelgroup.StatusEnabled)

	// 创建协议
	protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

	// 创建目标
	createTestTarget(t, client, protocol.ID, chID, "gpt-4", "openai/chat/completions", 1)

	// 创建多个绑定
	createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

	// 刷新快照
	_, err := svc.Refresh(ctx)
	require.NoError(t, err)

	t.Run("list models returns enabled bindings only", func(t *testing.T) {
		models, err := svc.ListModels(ctx, "list-models-adapter")
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"gpt-4"}, models)
	})

	t.Run("list models with disabled binding", func(t *testing.T) {
		// 创建另一个绑定但禁用
		createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-3.5-turbo", false)

		// 刷新
		_, err = svc.Refresh(ctx)
		require.NoError(t, err)

		models, err := svc.ListModels(ctx, "list-models-adapter")
		require.NoError(t, err)
		// 只有启用的绑定
		require.ElementsMatch(t, []string{"gpt-4"}, models)
	})

	t.Run("list models for non-existent adapter", func(t *testing.T) {
		_, err := svc.ListModels(ctx, "non-existent")
		require.Error(t, err)
	})
}

// TestAdapterService_Snapshot 测试快照功能。
func TestAdapterService_Snapshot(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("initial snapshot is empty", func(t *testing.T) {
		snapshot := svc.Snapshot()
		require.NotNil(t, snapshot)
		require.Equal(t, uint64(0), snapshot.Version)
		require.Empty(t, snapshot.Adapters)
	})

	t.Run("snapshot reflects refresh", func(t *testing.T) {
		// 设置渠道
		chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Snapshot Channel", "https://api.openai.com/v1", []string{"gpt-4"}, channel.StatusEnabled)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "Snapshot Channel", "https://api.openai.com/v1", []string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
		})

		// 创建适配器
		createTestAdapter(t, client, "snapshot-adapter", "openai/chat/completions", adapter.StatusEnabled)

		// 刷新
		result, err := svc.Refresh(ctx)
		require.NoError(t, err)
		require.Greater(t, result.SnapshotVersion, uint64(0))

		// 验证快照内容
		snapshot := svc.Snapshot()
		require.Equal(t, result.SnapshotVersion, snapshot.Version)
		require.Contains(t, snapshot.Adapters, "snapshot-adapter")
	})

	t.Run("snapshot is immutable after refresh", func(t *testing.T) {
		snapshot1 := svc.Snapshot()

		// 再次刷新
		_, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 快照应该是新的
		snapshot2 := svc.Snapshot()
		require.NotSame(t, snapshot1, snapshot2)
	})
}

// TestAdapterService_ProtocolAndTargetValidation 测试协议与目标校验。
func TestAdapterService_ProtocolAndTargetValidation(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("diagnostics for empty target model id", func(t *testing.T) {
		// 设置渠道
		chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Diag Channel 1", "https://api.openai.com/v1", []string{"gpt-4"}, channel.StatusEnabled)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "Diag Channel 1", "https://api.openai.com/v1", []string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
		})

		// 创建适配器
		adapterEnt := createTestAdapter(t, client, "diag-adapter-1", "openai/chat/completions", adapter.StatusEnabled)

		// 创建模型组
		modelGroup := createTestModelGroup(t, client, "diag-group-1", modelgroup.StatusEnabled)

		// 创建协议
		protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

		// 创建空目标模型 ID 的目标
		client.ModelGroupTarget.Create().
			SetModelGroupProtocolID(protocol.ID).
			SetChannelID(chID).
			SetTargetModelID("").
			SetOutboundAPIFormat("openai/chat/completions").
			SetPriority(1).
			SetEnabled(true).
			Save(ctx)

		// 创建绑定
		createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

		// 刷新
		result, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 验证诊断信息
		require.NotEmpty(t, result.Diagnostics)
		hasEmptyTargetModelDiag := false
		for _, diag := range result.Diagnostics {
			if diag.Reason == "target model id is empty" {
				hasEmptyTargetModelDiag = true
				break
			}
		}
		require.True(t, hasEmptyTargetModelDiag, "expected diagnostic for empty target model id")
	})

	t.Run("diagnostics for empty outbound api format", func(t *testing.T) {
		// 设置渠道
		chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Diag Channel 2", "https://api.openai.com/v1", []string{"gpt-4"}, channel.StatusEnabled)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "Diag Channel 2", "https://api.openai.com/v1", []string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
		})

		// 创建适配器
		adapterEnt := createTestAdapter(t, client, "diag-adapter-2", "openai/chat/completions", adapter.StatusEnabled)

		// 创建模型组
		modelGroup := createTestModelGroup(t, client, "diag-group-2", modelgroup.StatusEnabled)

		// 创建协议
		protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

		// 创建空 outbound api format 的目标
		client.ModelGroupTarget.Create().
			SetModelGroupProtocolID(protocol.ID).
			SetChannelID(chID).
			SetTargetModelID("gpt-4").
			SetOutboundAPIFormat("").
			SetPriority(1).
			SetEnabled(true).
			Save(ctx)

		// 创建绑定
		createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

		// 刷新
		result, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 验证诊断信息
		require.NotEmpty(t, result.Diagnostics)
		hasEmptyFormatDiag := false
		for _, diag := range result.Diagnostics {
			if diag.Reason == "outbound api format is empty" {
				hasEmptyFormatDiag = true
				break
			}
		}
		require.True(t, hasEmptyFormatDiag, "expected diagnostic for empty outbound api format")
	})

	t.Run("diagnostics for disabled channel", func(t *testing.T) {
		// 创建禁用的渠道
		chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Diag Channel 3", "https://api.openai.com/v1", []string{"gpt-4"}, channel.StatusDisabled)
		channelSvc.SetEnabledChannelsForTest([]*Channel{}) // 清空启用渠道

		// 创建适配器
		adapterEnt := createTestAdapter(t, client, "diag-adapter-3", "openai/chat/completions", adapter.StatusEnabled)

		// 创建模型组
		modelGroup := createTestModelGroup(t, client, "diag-group-3", modelgroup.StatusEnabled)

		// 创建协议
		protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

		// 创建目标
		createTestTarget(t, client, protocol.ID, chID, "gpt-4", "openai/chat/completions", 1)

		// 创建绑定
		createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

		// 刷新
		result, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 验证诊断信息
		require.NotEmpty(t, result.Diagnostics)
		hasChannelDiag := false
		for _, diag := range result.Diagnostics {
			if diag.Reason == "channel is not enabled or does not exist" {
				hasChannelDiag = true
				break
			}
		}
		require.True(t, hasChannelDiag, "expected diagnostic for disabled channel")
	})

	t.Run("diagnostics for channel without endpoint", func(t *testing.T) {
		// 创建渠道但不设置端点
		chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Diag Channel 4", "https://api.anthropic.com/v1", []string{"claude-3"}, channel.StatusEnabled)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "Diag Channel 4", "https://api.anthropic.com/v1", []string{"claude-3"}, []objects.ChannelEndpoint{createAdapterEndpoint("anthropic/messages", "/v1")}), // 不同的端点
		})

		// 创建适配器
		adapterEnt := createTestAdapter(t, client, "diag-adapter-4", "openai/chat/completions", adapter.StatusEnabled)

		// 创建模型组
		modelGroup := createTestModelGroup(t, client, "diag-group-4", modelgroup.StatusEnabled)

		// 创建协议
		protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

		// 创建目标，指向不存在于渠道端点的 api format
		createTestTarget(t, client, protocol.ID, chID, "gpt-4", "openai/chat/completions", 1)

		// 创建绑定
		createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

		// 刷新
		result, err := svc.Refresh(ctx)
		require.NoError(t, err)

		// 验证诊断信息
		require.NotEmpty(t, result.Diagnostics)
		hasEndpointDiag := false
		for _, diag := range result.Diagnostics {
			if diag.Reason == "channel does not expose the configured outbound api format" {
				hasEndpointDiag = true
				break
			}
		}
		require.True(t, hasEndpointDiag, "expected diagnostic for missing endpoint")
	})
}

// TestAdapterService_RuntimeStatus 测试运行时状态。
func TestAdapterService_RuntimeStatus(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("initial status is zero", func(t *testing.T) {
		status := svc.RuntimeStatus()
		require.Equal(t, uint64(0), status.SnapshotVersion)
		require.Empty(t, status.LastRefreshError)
	})

	t.Run("status reflects successful refresh", func(t *testing.T) {
		result, err := svc.Refresh(ctx)
		require.NoError(t, err)

		status := svc.RuntimeStatus()
		require.Equal(t, result.SnapshotVersion, status.SnapshotVersion)
		require.NotZero(t, status.RefreshedAt)
		require.Empty(t, status.LastRefreshError)
	})

	t.Run("status reflects failed refresh", func(t *testing.T) {
		// 设置无效数据触发错误
		createTestAdapter(t, client, "empty-name-adapter", "", adapter.StatusEnabled)

		// 刷新会失败
		_, err := svc.Refresh(ctx)
		require.Error(t, err)

		status := svc.RuntimeStatus()
		require.NotEmpty(t, status.LastRefreshError)
	})
}

// TestAdapterService_RefreshFailurePreservesSnapshot 测试刷新失败时保留旧快照。
func TestAdapterService_RefreshFailurePreservesSnapshot(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 初始刷新
	result1, err := svc.Refresh(ctx)
	require.NoError(t, err)
	snapshot1 := svc.Snapshot()

	// 设置渠道
	chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Preserve Channel", "https://api.openai.com/v1", []string{"gpt-4"}, channel.StatusEnabled)
	channelSvc.SetEnabledChannelsForTest([]*Channel{
		newTestBizChannel(chID, channel.TypeOpenai, "Preserve Channel", "https://api.openai.com/v1", []string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
	})

	// 成功刷新
	result2, err := svc.Refresh(ctx)
	require.NoError(t, err)
	snapshot2 := svc.Snapshot()

	// 版本应该增加
	require.Greater(t, result2.SnapshotVersion, result1.SnapshotVersion)
	require.NotSame(t, snapshot1, snapshot2)

	// 创建无效数据触发错误
	createTestAdapter(t, client, "", "openai/chat/completions", adapter.StatusEnabled)

	// 尝试刷新会失败
	_, err = svc.Refresh(ctx)
	require.Error(t, err)

	// 快照应该保持不变
	snapshot3 := svc.Snapshot()
	require.Same(t, snapshot2, snapshot3, "snapshot should be preserved after failed refresh")
}

// TestAdapterService_ListAdapters 测试 ListAdapters 方法。
func TestAdapterService_ListAdapters(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("list all adapters including disabled", func(t *testing.T) {
		// 创建多个适配器
		createTestAdapter(t, client, "enabled-adapter", "openai/chat/completions", adapter.StatusEnabled)
		createTestAdapter(t, client, "disabled-adapter", "openai/chat/completions", adapter.StatusDisabled)

		adapters, err := svc.ListAdapters(ctx)
		require.NoError(t, err)
		require.Len(t, adapters, 2)

		names := lo.Map(adapters, func(a AdapterInfo, _ int) string { return a.Name })
		require.Contains(t, names, "enabled-adapter")
		require.Contains(t, names, "disabled-adapter")
	})

	t.Run("list adapters with bindings", func(t *testing.T) {
		// 清理并重建
		client.Adapter.Delete().ExecX(ctx)
		client.AdapterModelBinding.Delete().ExecX(ctx)

		// 设置渠道
		chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Binding Channel", "https://api.openai.com/v1", []string{"gpt-4"}, channel.StatusEnabled)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "Binding Channel", "https://api.openai.com/v1", []string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
		})

		// 创建适配器
		adapterEnt := createTestAdapter(t, client, "binding-adapter", "openai/chat/completions", adapter.StatusEnabled)

		// 创建模型组
		modelGroup := createTestModelGroup(t, client, "binding-group", modelgroup.StatusEnabled)

		// 创建绑定
		createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

		adapters, err := svc.ListAdapters(ctx)
		require.NoError(t, err)
		require.Len(t, adapters, 1)

		adapterInfo := adapters[0]
		require.Equal(t, "binding-adapter", adapterInfo.Name)
		require.Len(t, adapterInfo.Bindings, 1)
		require.Equal(t, "gpt-4", adapterInfo.Bindings[0].SourceModelID)
	})
}

// TestAdapterService_UpdateAdapter 测试 UpdateAdapter 方法。
func TestAdapterService_UpdateAdapter(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("update adapter display name", func(t *testing.T) {
		adapterEnt := createTestAdapter(t, client, "update-test", "openai/chat/completions", adapter.StatusEnabled)

		result, err := svc.UpdateAdapter(ctx, "update-test", &UpdateAdapterParams{
			DisplayName: "Updated Display Name",
		})
		require.NoError(t, err)
		require.Equal(t, "Updated Display Name", result.DisplayName)

		// 验证数据库
		updated, err := client.Adapter.Get(ctx, adapterEnt.ID)
		require.NoError(t, err)
		require.Equal(t, "Updated Display Name", updated.DisplayName)
	})

	t.Run("update adapter status", func(t *testing.T) {
		createTestAdapter(t, client, "status-test", "openai/chat/completions", adapter.StatusEnabled)

		result, err := svc.UpdateAdapter(ctx, "status-test", &UpdateAdapterParams{
			Status: "disabled",
		})
		require.NoError(t, err)
		require.Equal(t, "disabled", result.Status)
	})

	t.Run("upsert non-existent adapter without inbound_api_format", func(t *testing.T) {
		// 创建时缺少 inbound_api_format 应报错
		_, err := svc.UpdateAdapter(ctx, "upsert-missing-format", &UpdateAdapterParams{
			DisplayName: "Test",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "inbound_api_format is required")
	})

	t.Run("upsert non-existent adapter creates it", func(t *testing.T) {
		info, err := svc.UpdateAdapter(ctx, "upsert-new-adapter", &UpdateAdapterParams{
			DisplayName:      "Upserted",
			InboundAPIFormat: "openai/chat/completions",
			Status:           "enabled",
		})
		require.NoError(t, err)
		require.Equal(t, "upsert-new-adapter", info.Name)
		require.Equal(t, "openai/chat/completions", info.InboundAPIFormat)
		require.Equal(t, "enabled", info.Status)
	})

	t.Run("update adapter bindings", func(t *testing.T) {
		// 清理并重建
		client.Adapter.Delete().ExecX(ctx)
		client.AdapterModelBinding.Delete().ExecX(ctx)

		_ = createTestAdapter(t, client, "binding-update-test", "openai/chat/completions", adapter.StatusEnabled)
		modelGroup := createTestModelGroup(t, client, "binding-update-group", modelgroup.StatusEnabled)

		// 初始没有绑定
		initial, err := svc.ListAdapters(ctx)
		require.NoError(t, err)
		require.Empty(t, initial[0].Bindings)

		// 更新绑定
		bindings := []BindingInput{
			{
				SourceModelID: "gpt-4",
				ModelGroupID:  modelGroup.ID,
				Enabled:       true,
			},
		}
		result, err := svc.UpdateAdapter(ctx, "binding-update-test", &UpdateAdapterParams{
			Bindings: bindings,
		})
		require.NoError(t, err)
		require.Len(t, result.Bindings, 1)
		require.Equal(t, "gpt-4", result.Bindings[0].SourceModelID)
	})
}

// TestAdapterService_ListModelGroups 测试 ListModelGroups 方法。
func TestAdapterService_ListModelGroups(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("list model groups", func(t *testing.T) {
		modelGroup := createTestModelGroup(t, client, "list-mg", modelgroup.StatusEnabled)

		groups, err := svc.ListModelGroups(ctx)
		require.NoError(t, err)
		require.Len(t, groups, 1)
		require.Equal(t, "list-mg", groups[0].Name)
		require.Equal(t, modelGroup.ID, groups[0].ID)
	})

	t.Run("list model groups with protocols", func(t *testing.T) {
		// 清理
		client.ModelGroup.Delete().ExecX(ctx)
		client.ModelGroupProtocol.Delete().ExecX(ctx)

		modelGroup := createTestModelGroup(t, client, "protocol-mg", modelgroup.StatusEnabled)
		protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

		groups, err := svc.ListModelGroups(ctx)
		require.NoError(t, err)
		require.Len(t, groups, 1)
		require.Len(t, groups[0].Protocols, 1)
		require.Equal(t, "openai/chat/completions", groups[0].Protocols[0].InboundAPIFormat)
		require.Equal(t, protocol.ID, groups[0].Protocols[0].ID)
	})
}

// TestAdapterService_UpdateModelGroup 测试 UpdateModelGroup 方法。
func TestAdapterService_UpdateModelGroup(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("update model group display name", func(t *testing.T) {
		modelGroup := createTestModelGroup(t, client, "mg-update-test", modelgroup.StatusEnabled)

		result, err := svc.UpdateModelGroup(ctx, "mg-update-test", &UpdateModelGroupParams{
			DisplayName: "Updated MG Name",
		})
		require.NoError(t, err)
		require.Equal(t, "Updated MG Name", result.DisplayName)

		// 验证数据库
		updated, err := client.ModelGroup.Get(ctx, modelGroup.ID)
		require.NoError(t, err)
		require.Equal(t, "Updated MG Name", updated.DisplayName)
	})

	t.Run("upsert non-existent model group creates it", func(t *testing.T) {
		info, err := svc.UpdateModelGroup(ctx, "upsert-new-group", &UpdateModelGroupParams{
			DisplayName: "Upserted Group",
			Status:      "enabled",
		})
		require.NoError(t, err)
		require.Equal(t, "upsert-new-group", info.Name)
		require.Equal(t, "enabled", info.Status)
	})

	t.Run("update with nil params", func(t *testing.T) {
		modelGroup := createTestModelGroup(t, client, "nil-params-test", modelgroup.StatusEnabled)

		_, err := svc.UpdateModelGroup(ctx, "nil-params-test", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "params is required")

		// 验证数据未被修改
		updated, err := client.ModelGroup.Get(ctx, modelGroup.ID)
		require.NoError(t, err)
		require.Equal(t, "", updated.DisplayName)
	})
}

// TestAdapterService_Refresh_WithDiagnostics 测试带诊断的刷新。
func TestAdapterService_Refresh_WithDiagnostics(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 设置渠道
	chID := createAdapterTestChannel(t, client, ctx, channel.TypeOpenai, "Diag Channel", "https://api.openai.com/v1", []string{"gpt-4"}, channel.StatusEnabled)
	channelSvc.SetEnabledChannelsForTest([]*Channel{
		newTestBizChannel(chID, channel.TypeOpenai, "Diag Channel", "https://api.openai.com/v1", []string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
	})

	// 创建适配器
	adapterEnt := createTestAdapter(t, client, "diag-adapter", "openai/chat/completions", adapter.StatusEnabled)

	// 创建模型组
	modelGroup := createTestModelGroup(t, client, "diag-group", modelgroup.StatusEnabled)

	// 创建协议
	protocol := createTestProtocol(t, client, modelGroup.ID, "openai/chat/completions")

	// 创建混合目标
	createTestTarget(t, client, protocol.ID, chID, "gpt-4", "openai/chat/completions", 1) // 有效
	createTestTarget(t, client, protocol.ID, chID, "", "openai/chat/completions", 2)      // 无效：空目标模型
	createTestTarget(t, client, protocol.ID, chID, "gpt-3.5", "", 3)                      // 无效：空 api format

	// 创建绑定
	createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-4", true)

	// 刷新
	result, err := svc.Refresh(ctx)
	require.NoError(t, err)

	// 验证诊断信息
	require.NotEmpty(t, result.Diagnostics)
	require.GreaterOrEqual(t, len(result.Diagnostics), 2)

	// 验证快照中的适配器仍然有效
	snapshot := svc.Snapshot()
	require.Contains(t, snapshot.Adapters, "diag-adapter")

	// 验证适配器的绑定
	adapterRuntime := snapshot.Adapters["diag-adapter"]
	require.NotNil(t, adapterRuntime)
	require.Contains(t, adapterRuntime.Bindings, "gpt-4")
}
