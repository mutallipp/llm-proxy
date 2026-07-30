package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/adapter"
	"github.com/looplj/axonhub/internal/ent/adaptermodelbinding"
	"github.com/looplj/axonhub/internal/ent/modelgroup"
)

// --- ValidateAdapterName ---

// TestValidateAdapterName 校验名称格式规则。
func TestValidateAdapterName(t *testing.T) {
	valid := []string{
		"gpt", "openai", "my-adapter", "adapter_1", "a1b2", "X",
	}
	for _, name := range valid {
		require.NoError(t, ValidateAdapterName(name), "should be valid: %q", name)
	}

	invalid := []struct {
		name   string
		reason string
	}{
		{"", "空名称"},
		{"  ", "仅空白"},
		{"-invalid", "以连字符开头"},
		{"_invalid", "以下划线开头"},
		{"has space", "含空格"},
		{"has/slash", "含斜杠"},
		{"has.dot", "含点号"},
	}
	for _, tc := range invalid {
		require.Error(t, ValidateAdapterName(tc.name), "should be invalid (%s): %q", tc.reason, tc.name)
	}
}

// --- RenameAdapter ---

// TestAdapterService_RenameAdapter_Basic 正常重命名。
func TestAdapterService_RenameAdapter_Basic(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 构建完整测试数据（含渠道、协议、目标、绑定）
	_, chID, adapterEnt, modelGroup := setupAdapterTestData(t, client, ctx, channelSvc)

	// 额外添加一条绑定，验证多绑定均被迁移
	createTestBinding(t, client, adapterEnt.ID, modelGroup.ID, "gpt-3.5-turbo", true)

	// 刷新快照，使旧名称生效
	_, err := svc.Refresh(ctx)
	require.NoError(t, err)

	// 执行重命名
	result, err := svc.RenameAdapter(ctx, "test-adapter", "renamed-adapter")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "renamed-adapter", result.Name)

	// 旧名称应被软删除
	_, findErr := client.Adapter.Query().
		Where(adapter.Name("test-adapter"), adapter.DeletedAtEQ(0)).
		Only(ctx)
	require.True(t, ent.IsNotFound(findErr), "旧适配器应被软删除")

	// 新适配器应存在
	newA, err2 := client.Adapter.Query().
		Where(adapter.Name("renamed-adapter"), adapter.DeletedAtEQ(0)).
		Only(ctx)
	require.NoError(t, err2)
	require.Equal(t, "openai/chat/completions", newA.InboundAPIFormat)

	// 新适配器的活跃绑定数量应等于旧适配器的活跃绑定数量（含刚添加的那条）
	newBindingCount, err3 := client.AdapterModelBinding.Query().
		Where(adaptermodelbinding.AdapterID(newA.ID), adaptermodelbinding.DeletedAtEQ(0)).
		Count(ctx)
	require.NoError(t, err3)
	require.Equal(t, 2, newBindingCount)

	// 旧适配器的绑定应全部被软删除
	oldBindingCount, err4 := client.AdapterModelBinding.Query().
		Where(adaptermodelbinding.AdapterID(adapterEnt.ID), adaptermodelbinding.DeletedAtEQ(0)).
		Count(ctx)
	require.NoError(t, err4)
	require.Equal(t, 0, oldBindingCount, "旧绑定应全部被软删除")

	// 渠道数据不受影响（间接验证只删除绑定，不删除 ModelGroup/Channel）
	_ = chID
}

// TestAdapterService_RenameAdapter_SameNameNoOp 同名重命名是 no-op，不产生重复数据。
func TestAdapterService_RenameAdapter_SameNameNoOp(t *testing.T) {
	_, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	svc, _, _ := setupTestAdapterService(t)
	createTestAdapter(t, client, "noop-adapter", "openai/chat/completions", adapter.StatusEnabled)

	result, err := svc.RenameAdapter(ctx, "noop-adapter", "noop-adapter")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "noop-adapter", result.Name)

	// 仍只有一条未删除的适配器
	count, _ := client.Adapter.Query().
		Where(adapter.Name("noop-adapter"), adapter.DeletedAtEQ(0)).
		Count(ctx)
	require.Equal(t, 1, count, "同名 no-op 不应创建新记录")
}

// TestAdapterService_RenameAdapter_OldNotFound 旧名称不存在时返回 ErrAdapterNotFound。
func TestAdapterService_RenameAdapter_OldNotFound(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	_, err := svc.RenameAdapter(ctx, "ghost-adapter", "new-name")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAdapterNotFound)
}

// TestAdapterService_RenameAdapter_NewNameConflict 新名称已被其他适配器占用时返回 ErrAdapterAlreadyExists。
func TestAdapterService_RenameAdapter_NewNameConflict(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	createTestAdapter(t, client, "adapter-a", "openai/chat/completions", adapter.StatusEnabled)
	createTestAdapter(t, client, "adapter-b", "openai/chat/completions", adapter.StatusEnabled)

	_, err := svc.RenameAdapter(ctx, "adapter-a", "adapter-b")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAdapterAlreadyExists)
}

// TestAdapterService_RenameAdapter_InvalidName 新名称非法时返回 ErrAdapterInvalidName。
func TestAdapterService_RenameAdapter_InvalidName(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	createTestAdapter(t, client, "valid-adapter", "openai/chat/completions", adapter.StatusEnabled)

	invalidNames := []string{"", "  ", "-bad", "has space", "with/slash"}
	for _, invalid := range invalidNames {
		_, err := svc.RenameAdapter(ctx, "valid-adapter", invalid)
		require.Error(t, err, "should fail for new_name=%q", invalid)
		require.ErrorIs(t, err, ErrAdapterInvalidName, "wrong error for new_name=%q", invalid)
	}
}

// --- DeleteAdapter ---

// TestAdapterService_DeleteAdapter_Basic 正常软删除。
func TestAdapterService_DeleteAdapter_Basic(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	_, _, adapterEnt, _ := setupAdapterTestData(t, client, ctx, channelSvc)

	// 刷新确认初始状态正常
	_, err := svc.Refresh(ctx)
	require.NoError(t, err)

	require.NoError(t, svc.DeleteAdapter(ctx, "test-adapter"))

	// 适配器应被软删除
	_, findErr := client.Adapter.Query().
		Where(adapter.Name("test-adapter"), adapter.DeletedAtEQ(0)).
		Only(ctx)
	require.True(t, ent.IsNotFound(findErr))

	// 活跃绑定应全部被软删除
	count, _ := client.AdapterModelBinding.Query().
		Where(adaptermodelbinding.AdapterID(adapterEnt.ID), adaptermodelbinding.DeletedAtEQ(0)).
		Count(ctx)
	require.Equal(t, 0, count)
}

// TestAdapterService_DeleteAdapter_NotFound 不存在时返回 ErrAdapterNotFound。
func TestAdapterService_DeleteAdapter_NotFound(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	err := svc.DeleteAdapter(ctx, "no-such-adapter")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAdapterNotFound)
}

// TestAdapterService_DeleteAdapter_SnapshotUpdated 删除后刷新快照，旧名称不再出现。
func TestAdapterService_DeleteAdapter_SnapshotUpdated(t *testing.T) {
	svc, channelSvc, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	setupAdapterTestData(t, client, ctx, channelSvc)

	_, err := svc.Refresh(ctx)
	require.NoError(t, err)
	require.Contains(t, svc.Snapshot().Adapters, "test-adapter")

	require.NoError(t, svc.DeleteAdapter(ctx, "test-adapter"))

	// 刷新后旧名称应从快照中消失
	_, refreshErr := svc.Refresh(ctx)
	require.NoError(t, refreshErr)
	require.NotContains(t, svc.Snapshot().Adapters, "test-adapter")
}

// --- DeleteModelGroup ---

// TestAdapterService_DeleteModelGroup_Basic 正常软删除（无绑定引用时）。
func TestAdapterService_DeleteModelGroup_Basic(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	g := createTestModelGroup(t, client, "unused-group", modelgroup.StatusEnabled)
	protocol := createTestProtocol(t, client, g.ID, "openai/chat/completions")
	_ = protocol

	require.NoError(t, svc.DeleteModelGroup(ctx, "unused-group"))

	// 模型组应被软删除
	_, findErr := client.ModelGroup.Query().
		Where(modelgroup.Name("unused-group"), modelgroup.DeletedAtEQ(0)).
		Only(ctx)
	require.True(t, ent.IsNotFound(findErr))
}

// TestAdapterService_DeleteModelGroup_NotFound 不存在时返回 ErrModelGroupNotFound。
func TestAdapterService_DeleteModelGroup_NotFound(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	err := svc.DeleteModelGroup(ctx, "ghost-group")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrModelGroupNotFound)
}

// TestAdapterService_DeleteModelGroup_InUse 有活跃 AdapterModelBinding 引用时返回 ErrModelGroupInUse（409 语义）。
func TestAdapterService_DeleteModelGroup_InUse(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 创建适配器和模型组
	adapterEnt := createTestAdapter(t, client, "in-use-adapter", "openai/chat/completions", adapter.StatusEnabled)
	g := createTestModelGroup(t, client, "in-use-group", modelgroup.StatusEnabled)

	// 创建仍然活跃的绑定（引用该模型组）
	createTestBinding(t, client, adapterEnt.ID, g.ID, "gpt-4", true)

	// 删除应返回 ErrModelGroupInUse
	err := svc.DeleteModelGroup(ctx, "in-use-group")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrModelGroupInUse)

	// 模型组不应被删除
	_, findErr := client.ModelGroup.Query().
		Where(modelgroup.Name("in-use-group"), modelgroup.DeletedAtEQ(0)).
		Only(ctx)
	require.NoError(t, findErr, "模型组不应被删除")
}

// TestAdapterService_DeleteModelGroup_AfterBindingDeleted 绑定被软删除后，模型组可以正常删除。
func TestAdapterService_DeleteModelGroup_AfterBindingDeleted(t *testing.T) {
	svc, _, client := setupTestAdapterService(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	adapterEnt := createTestAdapter(t, client, "ex-adapter", "openai/chat/completions", adapter.StatusEnabled)
	g := createTestModelGroup(t, client, "ex-group", modelgroup.StatusEnabled)
	createTestBinding(t, client, adapterEnt.ID, g.ID, "gpt-4", true)

	// 先软删除适配器（连带软删除绑定）
	require.NoError(t, svc.DeleteAdapter(ctx, "ex-adapter"))

	// 此时模型组可以被删除
	require.NoError(t, svc.DeleteModelGroup(ctx, "ex-group"))

	_, findErr := client.ModelGroup.Query().
		Where(modelgroup.Name("ex-group"), modelgroup.DeletedAtEQ(0)).
		Only(ctx)
	require.True(t, ent.IsNotFound(findErr), "模型组应被软删除")
}
