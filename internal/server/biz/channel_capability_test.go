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
