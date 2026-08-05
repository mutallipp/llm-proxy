package objects

import (
	"encoding/json"
	"testing"
)

func TestChannelProtocolCapabilitiesJSONRoundTrip(t *testing.T) {
	input := ChannelProtocolCapabilities{
		DeclaredProtocols: []string{"openai", "anthropic"},
		Models: []ChannelModelCapability{{
			ModelID:   "model-a",
			Protocols: []string{"openai"},
		}},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}

	var got ChannelProtocolCapabilities
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}

	if got.Models[0].ModelID != input.Models[0].ModelID || len(got.Models[0].Protocols) != 1 {
		t.Fatalf("round-trip mismatch: %#v", got)
	}
}

func TestModelAssociationAutoFieldsAreBackwardCompatible(t *testing.T) {
	var association ModelAssociation
	if err := json.Unmarshal([]byte(`{"type":"channel_model","channelModel":{"channelId":1,"modelId":"model-a"}}`), &association); err != nil {
		t.Fatalf("unmarshal association: %v", err)
	}
	if association.Auto || association.DisabledReason != "" {
		t.Fatalf("legacy association should have manual defaults: %#v", association)
	}

	data, err := json.Marshal(ModelAssociation{Auto: true, DisabledReason: "声明已撤销"})
	if err != nil {
		t.Fatalf("marshal association: %v", err)
	}
	if string(data) != `{"type":"","priority":0,"disabled":false,"auto":true,"disabledReason":"声明已撤销","channelModel":null,"channelRegex":null,"regex":null,"modelId":null,"channelTagsModel":null,"channelTagsRegex":null}` {
		t.Fatalf("unexpected JSON: %s", data)
	}
}

func TestSupportedInboundAPIFormat(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic"} {
		if !IsSupportedInboundAPIFormat(protocol) {
			t.Fatalf("expected supported protocol %q", protocol)
		}
	}
	if IsSupportedInboundAPIFormat("gemini") {
		t.Fatal("gemini must not be in the current protocol whitelist")
	}
}
