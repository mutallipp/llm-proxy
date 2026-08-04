package biz

import (
	"testing"

	"github.com/mutallipp/llm-proxy/internal/objects"
)

func TestReconcileAssociationsStateMachine(t *testing.T) {
	previous := objects.ChannelProtocolCapabilities{
		DeclaredProtocols: []string{"openai"},
		Models:            []objects.ChannelModelCapability{{ModelID: "model-a", Protocols: []string{"openai"}}},
	}
	current := previous
	local := map[string][]*objects.ModelAssociation{}

	added, stats := reconcileAssociations(7, objects.ChannelProtocolCapabilities{}, previous, local, local)
	if stats.AddedCount != 1 || len(added["openai"]) != 1 {
		t.Fatalf("expected one derived association: %#v, %#v", added, stats)
	}
	if !added["openai"][0].Auto || !added["openai"][0].Disabled {
		t.Fatalf("new association must be disabled and automatic: %#v", added["openai"][0])
	}

	added["openai"][0].Priority = 4
	repeated, stats := reconcileAssociations(7, previous, current, added, added)
	if stats.AddedCount != 0 || stats.AutoDisabledCount != 0 || repeated["openai"][0].Priority != 4 {
		t.Fatalf("idempotent reconciliation changed association: %#v, %#v", repeated, stats)
	}

	revoked, stats := reconcileAssociations(7, current, objects.ChannelProtocolCapabilities{}, repeated, repeated)
	if stats.AutoDisabledCount != 1 || !revoked["openai"][0].Disabled || revoked["openai"][0].DisabledReason != ProtocolPoolDisabledReasonCapabilityRevoked {
		t.Fatalf("revocation state mismatch: %#v, %#v", revoked, stats)
	}

	restored, stats := reconcileAssociations(7, objects.ChannelProtocolCapabilities{}, current, revoked, revoked)
	if stats.AutoRestoredCount != 1 || restored["openai"][0].DisabledReason != "" || !restored["openai"][0].Disabled {
		t.Fatalf("restoration state mismatch: %#v, %#v", restored, stats)
	}
}

func TestReconcileAssociationsDoesNotOverrideManualOrInherited(t *testing.T) {
	capabilities := objects.ChannelProtocolCapabilities{
		Models: []objects.ChannelModelCapability{{ModelID: "model-a", Protocols: []string{"openai"}}},
	}
	manual := &objects.ModelAssociation{
		Type:         "channel_model",
		ChannelModel: &objects.ChannelModelAssociation{ChannelID: 7, ModelID: "model-a"},
	}
	local := map[string][]*objects.ModelAssociation{"openai": {}}
	effective := map[string][]*objects.ModelAssociation{"openai": {manual}}

	got, stats := reconcileAssociations(7, objects.ChannelProtocolCapabilities{}, capabilities, local, effective)
	if stats.AddedCount != 0 || len(got["openai"]) != 0 {
		t.Fatalf("manual association should suppress derivation: %#v, %#v", got, stats)
	}
}

func TestReconcileAssociationsIncrementalDoesNotRevoke(t *testing.T) {
	local := map[string][]*objects.ModelAssociation{
		"openai": {{
			Type:           "channel_model",
			Auto:           true,
			Disabled:       true,
			DisabledReason: ProtocolPoolDisabledReasonCapabilityRevoked,
			ChannelModel:   &objects.ChannelModelAssociation{ChannelID: 7, ModelID: "model-a"},
		}},
	}
	current := objects.ChannelProtocolCapabilities{Models: []objects.ChannelModelCapability{{ModelID: "model-a", Protocols: []string{"openai"}}}}

	got, stats := reconcileAssociationsIncremental(7, current, local, local)
	if stats.AutoRestoredCount != 1 || got["openai"][0].DisabledReason != "" {
		t.Fatalf("incremental reconciliation should restore declaration: %#v, %#v", got, stats)
	}

	untouched, stats := reconcileAssociationsIncremental(7, objects.ChannelProtocolCapabilities{}, got, got)
	if stats.AutoDisabledCount != 0 || untouched["openai"][0].DisabledReason != "" {
		t.Fatalf("incremental reconciliation must not revoke: %#v, %#v", untouched, stats)
	}
}
