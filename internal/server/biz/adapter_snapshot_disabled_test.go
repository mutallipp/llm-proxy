package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/adapter"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/modelgroup"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

// TestAdapterService_DisabledAdapterWithActiveBindings 验证禁用的 Adapter 保留 active binding 时
// Refresh 必须成功，且运行时快照不包含该 Adapter。
//
// 复现场景：pi-openai disabled，pi-anthropic enabled，pi-openai 仍有 enabled binding，
// 启动时不应因此报错。
func TestAdapterService_DisabledAdapterWithActiveBindings(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 创建测试渠道
	chID := createAdapterTestChannel(t, client, ctx,
		channel.TypeOpenai, "ch-disabled-adapter", "https://api.openai.com/v1",
		[]string{"gpt-4"}, channel.StatusEnabled,
	)
	channelSvc.SetEnabledChannelsForTest([]*Channel{
		newTestBizChannel(chID, channel.TypeOpenai, "ch-disabled-adapter", "https://api.openai.com/v1",
			[]string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
	})

	// 创建 disabled Adapter（pi-openai 被禁用）
	disabledAdapter := createTestAdapter(t, client, "pi-openai", "openai/chat/completions", adapter.StatusDisabled)

	// 创建 enabled Adapter（pi-anthropic 正常运行）
	enabledAdapter := createTestAdapter(t, client, "pi-anthropic", "openai/chat/completions", adapter.StatusEnabled)

	// 为两个 adapter 准备一个共享 ModelGroup + Protocol + Target
	group := createTestModelGroup(t, client, "shared-group", modelgroup.StatusEnabled)
	protocol := createTestProtocol(t, client, group.ID, "openai/chat/completions")
	createTestTarget(t, client, protocol.ID, chID, "gpt-4", "openai/chat/completions", 1)

	// pi-openai（disabled）下仍有 enabled binding，代表"以后重新启用时保留配置"的合法状态
	createTestBinding(t, client, disabledAdapter.ID, group.ID, "gpt-4", true)

	// pi-anthropic（enabled）有正常 binding
	createTestBinding(t, client, enabledAdapter.ID, group.ID, "claude-3", true)

	// Refresh 必须成功，不能因 disabled adapter 下存在 active binding 而失败
	_, err := svc.Refresh(ctx)
	require.NoError(t, err, "Refresh 不应因 disabled Adapter 保留 active binding 而失败")

	snapshot := svc.Snapshot()

	// disabled adapter 不进入运行时
	require.NotContains(t, snapshot.Adapters, "pi-openai",
		"disabled Adapter 不应出现在运行时快照中")

	// enabled adapter 正常存在
	require.Contains(t, snapshot.Adapters, "pi-anthropic",
		"enabled Adapter 应存在于运行时快照中")
	require.Contains(t, snapshot.Adapters["pi-anthropic"].Bindings, "claude-3",
		"enabled Adapter 的 enabled binding 应进入运行时")
}

// TestAdapterService_DisabledModelGroupWithActiveProtocolsAndTargets 验证禁用的 ModelGroup 保留
// active protocol/target 时 Refresh 必须成功，运行时快照中该 Adapter 的 binding 为空。
func TestAdapterService_DisabledModelGroupWithActiveProtocolsAndTargets(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 创建测试渠道
	chID := createAdapterTestChannel(t, client, ctx,
		channel.TypeOpenai, "ch-disabled-mg", "https://api.openai.com/v1",
		[]string{"gpt-4"}, channel.StatusEnabled,
	)
	channelSvc.SetEnabledChannelsForTest([]*Channel{
		newTestBizChannel(chID, channel.TypeOpenai, "ch-disabled-mg", "https://api.openai.com/v1",
			[]string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
	})

	// 创建 enabled Adapter
	enabledAdapter := createTestAdapter(t, client, "adapter-with-disabled-group", "openai/chat/completions", adapter.StatusEnabled)

	// 创建 disabled ModelGroup——子配置应保留但不进入运行时
	disabledGroup := createTestModelGroup(t, client, "disabled-group", modelgroup.StatusDisabled)

	// disabled group 下仍有 enabled protocol 和 enabled target（合法的保留状态）
	disabledGroupProtocol := createTestProtocol(t, client, disabledGroup.ID, "openai/chat/completions")
	createTestTarget(t, client, disabledGroupProtocol.ID, chID, "gpt-4", "openai/chat/completions", 1)

	// enabled adapter 的 binding 指向 disabled group
	createTestBinding(t, client, enabledAdapter.ID, disabledGroup.ID, "gpt-4", true)

	// Refresh 必须成功
	_, err := svc.Refresh(ctx)
	require.NoError(t, err, "Refresh 不应因 disabled ModelGroup 保留 active protocol/target/binding 而失败")

	snapshot := svc.Snapshot()

	// enabled adapter 存在于运行时，但 binding 为空（因为引用的 group 被禁用）
	require.Contains(t, snapshot.Adapters, "adapter-with-disabled-group",
		"enabled Adapter 应存在于运行时快照中")
	adapterRuntime := snapshot.Adapters["adapter-with-disabled-group"]
	require.Empty(t, adapterRuntime.Bindings,
		"指向 disabled ModelGroup 的 binding 不应进入运行时")
	require.Empty(t, adapterRuntime.BindingOrder,
		"disabled ModelGroup 相关的 BindingOrder 应为空")
}

// TestAdapterService_DisabledProtocolWithActiveTargets 验证 disabled Protocol 下存在 active target
// 时 Refresh 必须成功。
func TestAdapterService_DisabledProtocolWithActiveTargets(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 创建测试渠道
	chID := createAdapterTestChannel(t, client, ctx,
		channel.TypeOpenai, "ch-disabled-proto", "https://api.openai.com/v1",
		[]string{"gpt-4"}, channel.StatusEnabled,
	)
	channelSvc.SetEnabledChannelsForTest([]*Channel{
		newTestBizChannel(chID, channel.TypeOpenai, "ch-disabled-proto", "https://api.openai.com/v1",
			[]string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
	})

	// 创建 enabled ModelGroup
	group := createTestModelGroup(t, client, "group-with-disabled-proto", modelgroup.StatusEnabled)

	// 创建 disabled Protocol（enabled=false）
	disabledProtocol, err := client.ModelGroupProtocol.Create().
		SetModelGroupID(group.ID).
		SetInboundAPIFormat("openai/chat/completions").
		SetEnabled(false).
		Save(ctx)
	require.NoError(t, err)

	// 在 disabled protocol 下创建 enabled target（保留配置的合法状态）
	_, err = client.ModelGroupTarget.Create().
		SetModelGroupProtocolID(disabledProtocol.ID).
		SetChannelID(chID).
		SetTargetModelID("gpt-4").
		SetOutboundAPIFormat("openai/chat/completions").
		SetPriority(1).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	// Refresh 必须成功——disabled protocol 下存在 active target 不应导致失败
	_, err = svc.Refresh(ctx)
	require.NoError(t, err, "Refresh 不应因 disabled Protocol 下存在 active target 而失败")
}

// TestAdapterService_EnabledParentBadReferenceStillFails 验证真实配置错误在 enabled parent 下仍会
// 导致 Refresh 失败，保证校验不被过度放松。
func TestAdapterService_EnabledParentBadReferenceStillFails(t *testing.T) {
	t.Run("enabled adapter with empty name fails", func(t *testing.T) {
		svc, _, client := setupTestAdapterService(t)
		defer client.Close()

		ctx := context.Background()
		ctx = ent.NewContext(ctx, client)
		ctx = authz.WithTestBypass(ctx)

		// enabled adapter 名称为空——这是真实配置错误，应失败
		createTestAdapter(t, client, "", "openai/chat/completions", adapter.StatusEnabled)

		_, err := svc.Refresh(ctx)
		require.Error(t, err, "enabled Adapter 名称为空应导致 Refresh 失败")
		require.Contains(t, err.Error(), "empty name")
	})

	t.Run("enabled binding with empty source model id fails", func(t *testing.T) {
		svc, channelSvc, client := setupTestAdapterService(t)
		defer client.Close()

		ctx := context.Background()
		ctx = ent.NewContext(ctx, client)
		ctx = authz.WithTestBypass(ctx)

		chID := createAdapterTestChannel(t, client, ctx,
			channel.TypeOpenai, "ch-empty-src", "https://api.openai.com/v1",
			[]string{"gpt-4"}, channel.StatusEnabled,
		)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "ch-empty-src", "https://api.openai.com/v1",
				[]string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
		})

		enabledAdapter := createTestAdapter(t, client, "bad-binding-adapter", "openai/chat/completions", adapter.StatusEnabled)
		group := createTestModelGroup(t, client, "bad-binding-group", modelgroup.StatusEnabled)

		// 创建 source_model_id 为空的 enabled binding——真实配置错误，应失败
		_, err := client.AdapterModelBinding.Create().
			SetAdapterID(enabledAdapter.ID).
			SetModelGroupID(group.ID).
			SetSourceModelID("").
			SetEnabled(true).
			Save(ctx)
		require.NoError(t, err)

		_, err = svc.Refresh(ctx)
		require.Error(t, err, "enabled binding source_model_id 为空应导致 Refresh 失败")
		require.Contains(t, err.Error(), "empty source model id")
	})

	t.Run("enabled binding in enabled group without matching protocol fails", func(t *testing.T) {
		svc, channelSvc, client := setupTestAdapterService(t)
		defer client.Close()

		ctx := context.Background()
		ctx = ent.NewContext(ctx, client)
		ctx = authz.WithTestBypass(ctx)

		chID := createAdapterTestChannel(t, client, ctx,
			channel.TypeOpenai, "ch-no-proto", "https://api.openai.com/v1",
			[]string{"claude-3"}, channel.StatusEnabled,
		)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "ch-no-proto", "https://api.openai.com/v1",
				[]string{"claude-3"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
		})

		// Adapter 使用 openai/chat/completions 格式
		enabledAdapter := createTestAdapter(t, client, "openai-format-adapter", "openai/chat/completions", adapter.StatusEnabled)

		// ModelGroup 只有 anthropic/messages 协议，无 openai/chat/completions
		group := createTestModelGroup(t, client, "anthropic-only-group", modelgroup.StatusEnabled)
		anthropicProtocol := createTestProtocol(t, client, group.ID, "anthropic/messages")
		createTestTarget(t, client, anthropicProtocol.ID, chID, "claude-3", "openai/chat/completions", 1)

		// enabled adapter → enabled group，但组缺少 adapter 所需协议
		createTestBinding(t, client, enabledAdapter.ID, group.ID, "gpt-4", true)

		_, err := svc.Refresh(ctx)
		require.Error(t, err, "enabled 组缺少 adapter 所需协议的 binding 应导致 Refresh 失败")
		require.Contains(t, err.Error(), "no enabled protocol")
	})

	t.Run("duplicate enabled bindings for same source model fail", func(t *testing.T) {
		svc, channelSvc, client := setupTestAdapterService(t)
		defer client.Close()

		ctx := context.Background()
		ctx = ent.NewContext(ctx, client)
		ctx = authz.WithTestBypass(ctx)

		chID := createAdapterTestChannel(t, client, ctx,
			channel.TypeOpenai, "ch-dup", "https://api.openai.com/v1",
			[]string{"gpt-4"}, channel.StatusEnabled,
		)
		channelSvc.SetEnabledChannelsForTest([]*Channel{
			newTestBizChannel(chID, channel.TypeOpenai, "ch-dup", "https://api.openai.com/v1",
				[]string{"gpt-4"}, []objects.ChannelEndpoint{createAdapterEndpoint("openai/chat/completions", "/v1")}),
		})

		enabledAdapter := createTestAdapter(t, client, "dup-adapter", "openai/chat/completions", adapter.StatusEnabled)
		group := createTestModelGroup(t, client, "dup-group", modelgroup.StatusEnabled)
		protocol := createTestProtocol(t, client, group.ID, "openai/chat/completions")
		createTestTarget(t, client, protocol.ID, chID, "gpt-4", "openai/chat/completions", 1)

		// 相同 source_model_id 的两个 enabled binding——真实配置错误，应失败
		createTestBinding(t, client, enabledAdapter.ID, group.ID, "gpt-4", true)
		createTestBinding(t, client, enabledAdapter.ID, group.ID, "gpt-4", true)

		_, err := svc.Refresh(ctx)
		require.Error(t, err, "相同 source_model_id 的重复 enabled binding 应导致 Refresh 失败")
		require.Contains(t, err.Error(), "duplicate enabled binding")
	})
}
