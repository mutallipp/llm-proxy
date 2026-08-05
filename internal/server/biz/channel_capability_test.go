package biz

import (
	"context"
	"slices"
	"testing"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	modelent "github.com/mutallipp/llm-proxy/internal/ent/model"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

func TestValidateProtocolCapabilities(t *testing.T) {
	openAIChannel := (&entChannelForCapabilityTest{Type: channel.TypeOpenai}).Channel()
	if err := validateProtocolCapabilities(openAIChannel, []string{"gemini"}, nil); err == nil {
		t.Fatal("unsupported protocol should be rejected")
	}
	if err := validateProtocolCapabilities(openAIChannel, []string{"anthropic"}, nil); err == nil {
		t.Fatal("protocol without endpoint support should be rejected")
	}
	if err := validateProtocolCapabilities(openAIChannel, []string{"openai"}, []objects.ChannelModelCapability{{ModelID: "model-a", Protocols: []string{"anthropic"}}}); err == nil {
		t.Fatal("model protocol outside channel declaration should be rejected")
	}
}

func TestMergeProtocolCapabilitiesPreservesUntouchedModelsAndPropagatesNewProtocol(t *testing.T) {
	previous := objects.ChannelProtocolCapabilities{
		DeclaredProtocols: []string{"openai"},
		Models: []objects.ChannelModelCapability{
			{ModelID: "model-a", Protocols: []string{"openai"}},
			{ModelID: "model-b", Protocols: []string{}},
		},
	}
	current := mergeProtocolCapabilities(previous, objects.ChannelProtocolCapabilities{DeclaredProtocols: []string{"openai", "anthropic"}, Models: nil})
	if len(current.Models) != 2 {
		t.Fatalf("untouched models were lost: %#v", current)
	}
	for _, modelCapability := range current.Models {
		if modelCapability.ModelID == "model-a" && len(modelCapability.Protocols) != 2 {
			t.Fatalf("new protocol was not propagated to model-a: %#v", modelCapability.Protocols)
		}
		if modelCapability.ModelID == "model-b" || modelCapability.ModelID == "model-a" {
			if !slices.Contains(modelCapability.Protocols, "anthropic") {
				t.Fatalf("new protocol was not propagated to %q: %#v", modelCapability.ModelID, modelCapability.Protocols)
			}
		}
	}

	exception := mergeProtocolCapabilities(current, objects.ChannelProtocolCapabilities{
		DeclaredProtocols: []string{"anthropic"},
		Models:            []objects.ChannelModelCapability{{ModelID: "model-b", Protocols: []string{}}},
	})
	for _, modelCapability := range exception.Models {
		if modelCapability.ModelID == "model-b" && len(modelCapability.Protocols) != 0 {
			t.Fatalf("explicit model exception was not preserved: %#v", modelCapability)
		}
	}
}

func TestSaveProtocolCapabilitiesDerivesMatchingModel(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()
	ctx := authz.WithTestBypass(context.Background())
	svc := NewChannelServiceForTest(client)

	logicalModel, err := client.Model.Create().
		SetDeveloper("provider").
		SetModelID("model-a").
		SetType(modelent.TypeChat).
		SetName("model-a").
		SetIcon("").
		SetGroup("").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("channel-a").
		SetCredentials(objects.ChannelCredentials{}).
		SetSupportedModels([]string{"model-a"}).
		SetDefaultTestModel("model-a").
		Save(ctx)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	payload, err := svc.SaveProtocolCapabilities(ctx, SaveChannelCapabilitiesInput{
		ChannelID:         objects.GUID{ID: ch.ID},
		DeclaredProtocols: []string{"openai"},
		Models:            []objects.ChannelModelCapability{{ModelID: "model-a", Protocols: []string{"openai"}}},
	})
	if err != nil {
		t.Fatalf("save capabilities: %v", err)
	}
	if payload.AddedCount != 1 || len(payload.UnmatchedModels) != 0 {
		t.Fatalf("unexpected save result: %#v", payload)
	}
	updated, err := client.Model.Get(ctx, logicalModel.ID)
	if err != nil {
		t.Fatalf("get model: %v", err)
	}
	association := updated.Settings.ProtocolPools["openai"][0]
	if !association.Auto || !association.Disabled || association.ChannelModel.ModelID != "model-a" {
		t.Fatalf("unexpected derived association: %#v", association)
	}
}

// entChannelForCapabilityTest 用于构造只包含端点相关字段的 Ent 渠道。
type entChannelForCapabilityTest struct {
	Type channel.Type
}

func (c *entChannelForCapabilityTest) Channel() *ent.Channel {
	return &ent.Channel{Type: c.Type}
}

// createLogicalModelForDerivationTest 创建用于派生测试的逻辑模型。
func createLogicalModelForDerivationTest(t *testing.T, client *ent.Client, ctx context.Context, modelID string) *ent.Model {
	t.Helper()
	logicalModel, err := client.Model.Create().
		SetDeveloper("provider").
		SetModelID(modelID).
		SetType(modelent.TypeChat).
		SetName(modelID).
		SetIcon("").
		SetGroup("").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create model %q: %v", modelID, err)
	}
	return logicalModel
}

// 回归：渠道声明多个模型时，每个逻辑模型的协议池只能出现自己同名的条目，
// 不能把渠道声明的其他模型写入目标模型池（E2E 发现：未匹配模型被错误写入他模型池）。
func TestSaveProtocolCapabilitiesDerivesOnlyOwnModelEntries(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()
	ctx := authz.WithTestBypass(context.Background())
	svc := NewChannelServiceForTest(client)

	modelA := createLogicalModelForDerivationTest(t, client, ctx, "model-a")
	modelB := createLogicalModelForDerivationTest(t, client, ctx, "model-b")
	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("channel-a").
		SetCredentials(objects.ChannelCredentials{}).
		SetSupportedModels([]string{"model-a", "model-b"}).
		SetDefaultTestModel("model-a").
		Save(ctx)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	payload, err := svc.SaveProtocolCapabilities(ctx, SaveChannelCapabilitiesInput{
		ChannelID:         objects.GUID{ID: ch.ID},
		DeclaredProtocols: []string{"openai"},
		Models: []objects.ChannelModelCapability{
			{ModelID: "model-a", Protocols: []string{"openai"}},
			{ModelID: "model-b", Protocols: []string{"openai"}},
			{ModelID: "orphan-x", Protocols: []string{"openai"}},
		},
	})
	if err != nil {
		t.Fatalf("save capabilities: %v", err)
	}
	if payload.AddedCount != 2 {
		t.Fatalf("addedCount = %d, want 2", payload.AddedCount)
	}
	if len(payload.UnmatchedModels) != 1 || payload.UnmatchedModels[0] != "orphan-x" {
		t.Fatalf("unexpected unmatched models: %#v", payload.UnmatchedModels)
	}

	for _, want := range []struct {
		model   *ent.Model
		modelID string
	}{
		{modelA, "model-a"},
		{modelB, "model-b"},
	} {
		updated, err := client.Model.Get(ctx, want.model.ID)
		if err != nil {
			t.Fatalf("get model %q: %v", want.modelID, err)
		}
		associations := updated.Settings.ProtocolPools["openai"]
		if len(associations) != 1 {
			t.Fatalf("model %q openai pool has %d associations, want 1: %#v", want.modelID, len(associations), associations)
		}
		if associations[0].ChannelModel.ModelID != want.modelID || !associations[0].Auto {
			t.Fatalf("model %q got unexpected association: %#v", want.modelID, associations[0])
		}
	}
}

// 回归：deriveModelAssociations 只能为目标逻辑模型添加同名条目，
// 不得把渠道声明的其他模型一并加入（E2E 发现：R10 addedCount 多算且写入无关物理关联）。
func TestDeriveModelAssociationsAddsOnlyTargetModelEntries(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()
	ctx := authz.WithTestBypass(context.Background())
	svc := NewChannelServiceForTest(client)

	modelA := createLogicalModelForDerivationTest(t, client, ctx, "model-a")
	createLogicalModelForDerivationTest(t, client, ctx, "model-b")
	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("channel-a").
		SetCredentials(objects.ChannelCredentials{}).
		SetSupportedModels([]string{"model-a", "model-b"}).
		SetDefaultTestModel("model-a").
		Save(ctx)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if _, err := svc.SaveProtocolCapabilities(ctx, SaveChannelCapabilitiesInput{
		ChannelID:         objects.GUID{ID: ch.ID},
		DeclaredProtocols: []string{"openai"},
		Models: []objects.ChannelModelCapability{
			{ModelID: "model-a", Protocols: []string{"openai"}},
			{ModelID: "model-b", Protocols: []string{"openai"}},
		},
	}); err != nil {
		t.Fatalf("save capabilities: %v", err)
	}

	// 清空 model-a 的派生条目后重新派生，验证增量模式只添加目标模型自己的条目。
	cleared, err := client.Model.UpdateOneID(modelA.ID).SetSettings(&objects.ModelSettings{}).Save(ctx)
	if err != nil {
		t.Fatalf("clear model-a settings: %v", err)
	}
	_ = cleared

	_, stats, err := svc.DeriveModelAssociations(ctx, modelA.ID)
	if err != nil {
		t.Fatalf("derive model associations: %v", err)
	}
	if stats.AddedCount != 1 {
		t.Fatalf("derive addedCount = %d, want 1", stats.AddedCount)
	}
	updated, err := client.Model.Get(ctx, modelA.ID)
	if err != nil {
		t.Fatalf("get model-a: %v", err)
	}
	associations := updated.Settings.ProtocolPools["openai"]
	if len(associations) != 1 || associations[0].ChannelModel.ModelID != "model-a" {
		t.Fatalf("model-a pool got unexpected associations: %#v", associations)
	}
}
