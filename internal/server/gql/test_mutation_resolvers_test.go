package gql

import (
	"context"
	"testing"

	"entgo.io/ent/privacy"
	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/scopes"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

func TestMutationResolver_TestAdapterRequiresWriteChannelsScope(t *testing.T) {
	resolver := &mutationResolver{&Resolver{}}
	input := TestAdapterInput{Adapter: "adapter", ModelID: "model"}

	_, err := resolver.TestAdapter(context.Background(), input)
	require.ErrorIs(t, err, privacy.Deny)

	ctx := contexts.WithUser(context.Background(), &ent.User{
		Scopes: []string{string(scopes.ScopeWriteChannels)},
	})
	_, err = resolver.TestAdapter(ctx, input)
	require.Error(t, err)
	require.NotErrorIs(t, err, privacy.Deny)
}

func TestMutationResolver_TestChannelRejectsDisabledChannelBeforeOrchestrator(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	entity, err := client.Channel.Create().
		SetName("disabled-test-channel").
		SetType(channel.TypeOpenaiFake).
		SetBaseURL("http://127.0.0.1").
		SetCredentials(objects.ChannelCredentials{}).
		SetSupportedModels([]string{"test-model"}).
		SetDefaultTestModel("test-model").
		SetStatus(channel.StatusDisabled).
		Save(ctx)
	require.NoError(t, err)

	channelService := biz.NewChannelService(biz.ChannelServiceParams{Ent: client})
	resolver := &mutationResolver{&Resolver{channelService: channelService}}
	_, err = resolver.TestChannel(ctx, TestChannelInput{
		ChannelID: objects.GUID{Type: "Channel", ID: entity.ID},
		ModelID:   new("test-model"),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "not enabled")
	require.Equal(t, 0, client.Request.Query().CountX(ctx))
}
