package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/llm"
)

func setupAdapterSelectorTest(t *testing.T) (*AdapterCandidateSelector, *biz.ChannelService, *ent.Client) {
	t.Helper()
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	channelSvc := biz.NewChannelServiceForTest(client)
	adapterSvc := biz.NewAdapterService(biz.AdapterServiceParams{Ent: client, ChannelService: channelSvc})
	return NewAdapterCandidateSelector(adapterSvc, channelSvc), channelSvc, client
}

func newSelectorTestBizChannel(id int, typ channel.Type, name, baseURL string, models []string, endpoints []objects.ChannelEndpoint) *biz.Channel {
	return &biz.Channel{Channel: &ent.Channel{ID: id, Type: typ, Name: name, BaseURL: baseURL, SupportedModels: models, DefaultTestModel: models[0], Endpoints: endpoints}}
}

func selectorEndpoint(format string) objects.ChannelEndpoint {
	return objects.ChannelEndpoint{APIFormat: format, Path: "/v1"}
}

func selectorModel(pools map[string][]*objects.ModelAssociation) *objects.RuntimeModel {
	return &objects.RuntimeModel{ModelID: "gpt-4", Settings: &objects.ModelSettings{ProtocolPools: pools}}
}

func selectorAssociation(channelID int, modelID string, priority int) *objects.ModelAssociation {
	return &objects.ModelAssociation{Type: "channel_model", Priority: priority, ChannelModel: &objects.ChannelModelAssociation{ChannelID: channelID, ModelID: modelID}}
}

func selectorAdapter(format string, model *objects.RuntimeModel) *objects.RuntimeAdapter {
	return &objects.RuntimeAdapter{ID: 1, Name: "test-adapter", InboundAPIFormat: format, Bindings: map[string]*objects.RuntimeAdapterBinding{"gpt-4": {SourceModelID: "gpt-4", Model: model, Enabled: true}}}
}

func selectorRequest(format string) *llm.Request {
	return &llm.Request{Model: "gpt-4", APIFormat: format}
}

func TestAdapterCandidateSelector_Select(t *testing.T) {
	selector, channelSvc, client := setupAdapterSelectorTest(t)
	defer client.Close()
	ctx := ent.NewContext(context.Background(), client)

	t.Run("nil request returns error", func(t *testing.T) {
		candidates, err := selector.Select(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, candidates)
	})
	t.Run("missing runtime adapter context", func(t *testing.T) {
		candidates, err := selector.Select(ctx, selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Nil(t, candidates)
	})
	t.Run("source model not bound", func(t *testing.T) {
		adapter := selectorAdapter("openai/chat/completions", nil)
		adapter.Bindings = map[string]*objects.RuntimeAdapterBinding{}
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Nil(t, candidates)
	})
	t.Run("missing inbound pool does not fallback", func(t *testing.T) {
		adapter := selectorAdapter("openai/chat/completions", selectorModel(map[string][]*objects.ModelAssociation{"anthropic/messages": {selectorAssociation(1, "claude", 1)}}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no protocol pool")
		assert.Nil(t, candidates)
	})
	t.Run("no usable target", func(t *testing.T) {
		adapter := selectorAdapter("openai/chat/completions", selectorModel(map[string][]*objects.ModelAssociation{"openai/chat/completions": {}}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no usable target")
		assert.Nil(t, candidates)
	})

	channelSvc.SetEnabledChannelsForTest([]*biz.Channel{
		newSelectorTestBizChannel(1, channel.TypeOpenai, "OpenAI", "https://openai.example", []string{"gpt-4"}, []objects.ChannelEndpoint{selectorEndpoint("openai/chat/completions")}),
		newSelectorTestBizChannel(2, channel.TypeAnthropic, "Anthropic", "https://anthropic.example", []string{"claude"}, []objects.ChannelEndpoint{selectorEndpoint("anthropic/messages")}),
	})
	t.Run("openai adapter cannot select anthropic pool", func(t *testing.T) {
		model := selectorModel(map[string][]*objects.ModelAssociation{"anthropic/messages": {selectorAssociation(2, "claude", 1)}})
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, selectorAdapter("openai/chat/completions", model)), selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no protocol pool")
		assert.Nil(t, candidates)
	})
	t.Run("same model is reused by adapters with isolated pools", func(t *testing.T) {
		model := selectorModel(map[string][]*objects.ModelAssociation{
			"openai/chat/completions": {selectorAssociation(1, "gpt-4", 1)},
			"anthropic/messages":      {selectorAssociation(2, "claude", 2)},
		})
		openai, err := selector.Select(contexts.WithRuntimeAdapter(ctx, selectorAdapter("openai/chat/completions", model)), selectorRequest("openai/chat/completions"))
		require.NoError(t, err)
		require.Len(t, openai, 1)
		assert.Equal(t, "gpt-4", openai[0].Models[0].ActualModel)
		anthropic, err := selector.Select(contexts.WithRuntimeAdapter(ctx, selectorAdapter("anthropic/messages", model)), selectorRequest("anthropic/messages"))
		require.NoError(t, err)
		require.Len(t, anthropic, 1)
		assert.Equal(t, "claude", anthropic[0].Models[0].ActualModel)
	})
}

func TestAdapterCandidateSelector_ResolveAdapter(t *testing.T) {
	selector, _, client := setupAdapterSelectorTest(t)
	defer client.Close()
	ctx := ent.NewContext(context.Background(), client)
	adapter := &objects.RuntimeAdapter{Name: "context-adapter"}
	result, err := selector.resolveAdapter(contexts.WithRuntimeAdapter(ctx, adapter))
	require.NoError(t, err)
	assert.Equal(t, "context-adapter", result.Name)
	result, err = selector.resolveAdapter(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestHasOutboundEndpoint(t *testing.T) {
	channel := &biz.Channel{Channel: &ent.Channel{Endpoints: []objects.ChannelEndpoint{{APIFormat: "openai/chat/completions", Path: "/v1"}}}}
	assert.True(t, hasOutboundEndpoint(channel, "openai/chat/completions"))
	assert.False(t, hasOutboundEndpoint(channel, "anthropic/messages"))
}
