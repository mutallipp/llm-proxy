package biz

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/adapter"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/ent/model"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

func TestRefreshAdapterSnapshotOnStart(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:adapter-refresh-privacy?mode=memory&_fk=1")
	defer client.Close()

	setupCtx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	adapterEntity, err := client.Adapter.Create().
		SetName("test-adapter").
		SetInboundAPIFormat("openai").
		SetStatus(adapter.StatusEnabled).
		Save(setupCtx)
	require.NoError(t, err)

	protocolPools := map[string][]*objects.ModelAssociation{
		"openai": {
			{
				Type:    "model",
				ModelID: &objects.ModelIDAssociation{ModelID: "gpt-4"},
			},
		},
		"anthropic": {
			{
				Type:    "model",
				ModelID: &objects.ModelIDAssociation{ModelID: "claude-3"},
			},
		},
	}
	modelEntity, err := client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-4").
		SetType(model.TypeChat).
		SetName("GPT-4").
		SetIcon("openai").
		SetGroup("gpt").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{ProtocolPools: protocolPools}).
		SetStatus(model.StatusEnabled).
		Save(setupCtx)
	require.NoError(t, err)

	_, err = client.AdapterModelBinding.Create().
		SetAdapterID(adapterEntity.ID).
		SetSourceModelID("chat-alias").
		SetModelID(modelEntity.ID).
		SetEnabled(true).
		Save(setupCtx)
	require.NoError(t, err)

	svc := NewAdapterService(AdapterServiceParams{Ent: client})

	_, err = svc.Refresh(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "no user in context")
	require.Equal(t, uint64(0), svc.Snapshot().Version)

	err = refreshAdapterSnapshotOnStart(context.Background(), svc)
	require.NoError(t, err)

	snapshot := svc.Snapshot()
	require.Equal(t, uint64(1), snapshot.Version)
	runtimeAdapter, ok := snapshot.Adapters["test-adapter"]
	require.True(t, ok)
	runtimeBinding, ok := runtimeAdapter.Bindings["chat-alias"]
	require.True(t, ok)
	require.Equal(t, "chat-alias", runtimeBinding.SourceModelID)
	require.Equal(t, "gpt-4", runtimeBinding.Model.ModelID)
	require.Equal(t, protocolPools, runtimeBinding.Model.Settings.ProtocolPools)

	_, err = svc.Refresh(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "no user in context")
	require.Equal(t, uint64(1), svc.Snapshot().Version)
	require.False(t, authz.IsBypassActive(context.Background()))
}
