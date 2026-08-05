package datamigrate

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/system"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

const (
	legacyOpenAIProtocol    = "openai"
	openAIResponsesProtocol = "openai_responses"
)

type V1_0_0_Beta7 struct{}

func NewV1_0_0_Beta7() DataMigrator {
	return &V1_0_0_Beta7{}
}

func (v *V1_0_0_Beta7) Version() string {
	return "v1.0.0-beta7"
}

// Migrate 将旧的 OpenAI 协议族拆分为 Chat Completions 和 Responses 两个协议池。
func (v *V1_0_0_Beta7) Migrate(ctx context.Context, client *ent.Client) (err error) {
	ctx = authz.WithSystemBypass(ctx, "database-migrate")
	ctx, tx, err := client.OpenTx(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err := migrateChannelCapabilities(ctx); err != nil {
		return err
	}
	if err := migrateModelProtocolPools(ctx); err != nil {
		return err
	}
	if err := migrateDeveloperProtocolPools(ctx); err != nil {
		return err
	}

	return tx.Commit()
}

func migrateChannelCapabilities(ctx context.Context) error {
	channels, err := ent.FromContext(ctx).Channel.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query channels for protocol migration: %w", err)
	}

	for _, channel := range channels {
		endpoints := (&biz.Channel{Channel: channel}).ResolveEndpoints()
		migrated, changed := migrateChannelProtocolCapabilities(channel.ProtocolCapabilities, endpoints)
		if !changed {
			continue
		}
		if _, err := ent.FromContext(ctx).Channel.UpdateOneID(channel.ID).SetProtocolCapabilities(migrated).Save(ctx); err != nil {
			return fmt.Errorf("failed to migrate channel %d protocol capabilities: %w", channel.ID, err)
		}
	}

	return nil
}

func migrateChannelProtocolCapabilities(capabilities objects.ChannelProtocolCapabilities, endpoints []objects.ChannelEndpoint) (objects.ChannelProtocolCapabilities, bool) {
	if !containsString(capabilities.DeclaredProtocols, legacyOpenAIProtocol) {
		return capabilities, false
	}

	supportsChat := endpointSupportsProtocol(endpoints, "openai/chat_completions")
	supportsResponses := endpointSupportsProtocol(endpoints, "openai/responses")
	declared := make([]string, 0, len(capabilities.DeclaredProtocols)+1)
	for _, protocol := range capabilities.DeclaredProtocols {
		if protocol != legacyOpenAIProtocol {
			appendUniqueString(&declared, protocol)
			continue
		}
		if supportsChat {
			appendUniqueString(&declared, legacyOpenAIProtocol)
		}
		if supportsResponses {
			appendUniqueString(&declared, openAIResponsesProtocol)
		}
	}

	models := make([]objects.ChannelModelCapability, 0, len(capabilities.Models))
	for _, modelCapability := range capabilities.Models {
		migrated := modelCapability
		migrated.Protocols = nil
		for _, protocol := range modelCapability.Protocols {
			if protocol != legacyOpenAIProtocol {
				appendUniqueString(&migrated.Protocols, protocol)
				continue
			}
			if supportsChat && containsString(declared, legacyOpenAIProtocol) {
				appendUniqueString(&migrated.Protocols, legacyOpenAIProtocol)
			}
			if supportsResponses && containsString(declared, openAIResponsesProtocol) {
				appendUniqueString(&migrated.Protocols, openAIResponsesProtocol)
			}
		}
		models = append(models, migrated)
	}

	migrated := objects.ChannelProtocolCapabilities{DeclaredProtocols: declared, Models: models}
	return migrated, !reflect.DeepEqual(capabilities, migrated)
}

func endpointSupportsProtocol(endpoints []objects.ChannelEndpoint, prefix string) bool {
	for _, endpoint := range endpoints {
		if strings.HasPrefix(endpoint.APIFormat, prefix) {
			return true
		}
	}

	return false
}

func migrateModelProtocolPools(ctx context.Context) error {
	models, err := ent.FromContext(ctx).Model.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query models for protocol migration: %w", err)
	}

	for _, model := range models {
		migrated, changed, err := copyProtocolPool(model.Settings)
		if err != nil {
			return fmt.Errorf("failed to clone model %d settings: %w", model.ID, err)
		}
		if !changed {
			continue
		}
		if _, err := ent.FromContext(ctx).Model.UpdateOneID(model.ID).SetSettings(migrated).Save(ctx); err != nil {
			return fmt.Errorf("failed to migrate model %d protocol pools: %w", model.ID, err)
		}
	}

	return nil
}

func migrateDeveloperProtocolPools(ctx context.Context) error {
	stored, err := ent.FromContext(ctx).System.Query().Where(system.KeyEQ(biz.SystemKeyModelSettings)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to query system model settings: %w", err)
	}

	var settings biz.SystemModelSettings
	if err := json.Unmarshal([]byte(stored.Value), &settings); err != nil {
		return fmt.Errorf("failed to unmarshal system model settings: %w", err)
	}

	changed := false
	for _, developer := range settings.DeveloperSettings {
		if developer == nil {
			continue
		}
		developerChanged, err := copyProtocolPoolMap(developer.ProtocolPools)
		if err != nil {
			return fmt.Errorf("failed to migrate developer %q protocol pools: %w", developer.Developer, err)
		}
		changed = changed || developerChanged
	}
	if !changed {
		return nil
	}

	encoded, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal migrated system model settings: %w", err)
	}
	if _, err := ent.FromContext(ctx).System.UpdateOneID(stored.ID).SetValue(string(encoded)).Save(ctx); err != nil {
		return fmt.Errorf("failed to save migrated developer protocol pools: %w", err)
	}

	return nil
}

func copyProtocolPool(settings *objects.ModelSettings) (*objects.ModelSettings, bool, error) {
	if settings == nil {
		return settings, false, nil
	}

	migrated, err := cloneJSON(settings)
	if err != nil {
		return nil, false, err
	}
	changed, err := copyProtocolPoolMap(migrated.ProtocolPools)
	if err != nil {
		return nil, false, err
	}

	return migrated, changed, nil
}

func copyProtocolPoolMap(pools map[string][]*objects.ModelAssociation) (bool, error) {
	legacyAssociations, ok := pools[legacyOpenAIProtocol]
	if !ok || len(legacyAssociations) == 0 {
		return false, nil
	}

	changed := false
	for _, association := range legacyAssociations {
		if association == nil || hasEquivalentAssociation(pools[openAIResponsesProtocol], association) {
			continue
		}
		clone, err := cloneJSON(association)
		if err != nil {
			return false, err
		}
		pools[openAIResponsesProtocol] = append(pools[openAIResponsesProtocol], clone)
		changed = true
	}

	return changed, nil
}

func hasEquivalentAssociation(associations []*objects.ModelAssociation, wanted *objects.ModelAssociation) bool {
	wantedFingerprint := associationFingerprint(wanted)
	for _, association := range associations {
		if associationFingerprint(association) == wantedFingerprint {
			return true
		}
	}

	return false
}

func associationFingerprint(association *objects.ModelAssociation) string {
	if association == nil {
		return "<nil>"
	}
	clone := *association
	clone.Priority = 0
	clone.Disabled = false
	clone.Auto = false
	clone.DisabledReason = ""
	encoded, err := json.Marshal(clone)
	if err != nil {
		return fmt.Sprintf("%#v", clone)
	}

	return string(encoded)
}

func cloneJSON[T any](value T) (T, error) {
	var clone T
	encoded, err := json.Marshal(value)
	if err != nil {
		return clone, err
	}
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return clone, err
	}

	return clone, nil
}

func appendUniqueString(values *[]string, value string) {
	if !containsString(*values, value) {
		*values = append(*values, value)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}

	return false
}

var _ DataMigrator = (*V1_0_0_Beta7)(nil)
