package gql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

func setupTestSystemModelSettingsResolver(t *testing.T) (*mutationResolver, context.Context, *ent.Client) {
	t.Helper()

	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	systemService := biz.NewSystemService(biz.SystemServiceParams{Ent: client})

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	resolver := &Resolver{
		client:        client,
		systemService: systemService,
	}

	return &mutationResolver{resolver}, ctx, client
}

func TestUpdateSystemModelSettings_PreservesDeveloperSettingsWhenOmitted(t *testing.T) {
	mutationResolver, ctx, client := setupTestSystemModelSettingsResolver(t)
	defer client.Close()

	err := mutationResolver.systemService.SetModelSettings(ctx, biz.SystemModelSettings{
		QueryAllChannelModels: true,
		DeveloperSettings: []*biz.DeveloperModelSettings{
			{
				Developer: "openai",
				ProtocolPools: map[string][]*objects.ModelAssociation{"openai": {
					{
						Type:         "channel_model",
						ChannelModel: &objects.ChannelModelAssociation{ChannelID: 10},
					},
				}},
			},
		},
	})
	require.NoError(t, err)

	ok, err := mutationResolver.UpdateSystemModelSettings(ctx, biz.SystemModelSettings{
		QueryAllChannelModels: false,
	})
	require.NoError(t, err)
	require.True(t, ok)

	settings, err := mutationResolver.systemService.ModelSettings(ctx)
	require.NoError(t, err)
	require.Len(t, settings.DeveloperSettings, 1)
	require.Equal(t, "openai", settings.DeveloperSettings[0].Developer)
	require.False(t, settings.QueryAllChannelModels)
}

func TestUpdateSystemModelSettings_AllowsExplicitDeveloperSettingsClear(t *testing.T) {
	mutationResolver, ctx, client := setupTestSystemModelSettingsResolver(t)
	defer client.Close()

	err := mutationResolver.systemService.SetModelSettings(ctx, biz.SystemModelSettings{
		DeveloperSettings: []*biz.DeveloperModelSettings{
			{
				Developer: "openai",
				ProtocolPools: map[string][]*objects.ModelAssociation{"openai": {
					{
						Type:         "channel_model",
						ChannelModel: &objects.ChannelModelAssociation{ChannelID: 10},
					},
				}},
			},
		},
	})
	require.NoError(t, err)

	emptyDeveloperSettings := []*biz.DeveloperModelSettings{}
	ok, err := mutationResolver.UpdateSystemModelSettings(ctx, biz.SystemModelSettings{
		DeveloperSettings: emptyDeveloperSettings,
	})
	require.NoError(t, err)
	require.True(t, ok)

	settings, err := mutationResolver.systemService.ModelSettings(ctx)
	require.NoError(t, err)
	require.Empty(t, settings.DeveloperSettings)
}
