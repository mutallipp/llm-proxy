package datamigrate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/ent/migrate/datamigrate"
	modelent "github.com/mutallipp/llm-proxy/internal/ent/model"
	"github.com/mutallipp/llm-proxy/internal/ent/system"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

func TestV1_0_0_Beta7_SplitsLegacyOpenAIProtocolPools(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:protocol-pool-split?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())
	association := &objects.ModelAssociation{
		Type:         "channel_model",
		ChannelModel: &objects.ChannelModelAssociation{ChannelID: 1, ModelID: "model-a"},
	}
	logicalModel := client.Model.Create().
		SetDeveloper("provider").
		SetModelID("model-a").
		SetType(modelent.TypeChat).
		SetName("model-a").
		SetIcon("").
		SetGroup("").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{ProtocolPools: map[string][]*objects.ModelAssociation{
			"openai": {association},
		}}).
		SaveX(ctx)
	client.Channel.Create().
		SetType(channel.TypeOpenaiResponses).
		SetName("responses-channel").
		SetCredentials(objects.ChannelCredentials{}).
		SetSupportedModels([]string{"model-a"}).
		SetDefaultTestModel("model-a").
		SetProtocolCapabilities(objects.ChannelProtocolCapabilities{
			DeclaredProtocols: []string{"openai"},
			Models:            []objects.ChannelModelCapability{{ModelID: "model-a", Protocols: []string{"openai"}}},
		}).
		SaveX(ctx)

	settingsJSON, err := json.Marshal(biz.SystemModelSettings{
		DeveloperSettings: []*biz.DeveloperModelSettings{{
			Developer: "provider",
			ProtocolPools: map[string][]*objects.ModelAssociation{
				"openai": {{
					Type:         "channel_model",
					ChannelModel: &objects.ChannelModelAssociation{ChannelID: 1},
				}},
			},
		}},
	})
	require.NoError(t, err)
	client.System.Create().
		SetKey(biz.SystemKeyModelSettings).
		SetValue(string(settingsJSON)).
		SaveX(ctx)

	migration := datamigrate.NewV1_0_0_Beta7()
	require.NoError(t, migration.Migrate(ctx, client))
	require.NoError(t, migration.Migrate(ctx, client))

	migratedChannel := client.Channel.Query().OnlyX(ctx)
	require.Equal(t, []string{"openai_responses"}, migratedChannel.ProtocolCapabilities.DeclaredProtocols)
	require.Equal(t, []string{"openai_responses"}, migratedChannel.ProtocolCapabilities.Models[0].Protocols)

	migratedModel := client.Model.GetX(ctx, logicalModel.ID)
	require.Len(t, migratedModel.Settings.ProtocolPools["openai"], 1)
	require.Len(t, migratedModel.Settings.ProtocolPools["openai_responses"], 1)

	migratedSystem := client.System.Query().Where(system.KeyEQ(biz.SystemKeyModelSettings)).OnlyX(ctx)
	var migratedSettings biz.SystemModelSettings
	require.NoError(t, json.Unmarshal([]byte(migratedSystem.Value), &migratedSettings))
	require.Len(t, migratedSettings.DeveloperSettings[0].ProtocolPools["openai"], 1)
	require.Len(t, migratedSettings.DeveloperSettings[0].ProtocolPools["openai_responses"], 1)
}
