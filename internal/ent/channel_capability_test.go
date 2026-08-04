package ent_test

import (
	"context"
	"testing"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

func TestChannelProtocolCapabilitiesRoundTrip(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()
	ctx := authz.WithTestBypass(context.Background())

	want := objects.ChannelProtocolCapabilities{
		DeclaredProtocols: []string{"openai"},
		Models:            []objects.ChannelModelCapability{{ModelID: "model-a", Protocols: []string{"openai"}}},
	}
	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("channel-a").
		SetCredentials(objects.ChannelCredentials{}).
		SetSupportedModels([]string{"model-a"}).
		SetDefaultTestModel("model-a").
		SetProtocolCapabilities(want).
		Save(ctx)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	got, err := client.Channel.Get(ctx, ch.ID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	if len(got.ProtocolCapabilities.Models) != 1 || got.ProtocolCapabilities.Models[0].ModelID != "model-a" {
		t.Fatalf("unexpected capabilities: %#v", got.ProtocolCapabilities)
	}
}
