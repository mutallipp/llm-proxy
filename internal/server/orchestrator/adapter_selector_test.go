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

func selectorEndpoint(format llm.APIFormat) objects.ChannelEndpoint {
	return objects.ChannelEndpoint{APIFormat: format.String(), Path: "/v1"}
}

func selectorModel(pools map[string][]*objects.ModelAssociation) *objects.RuntimeModel {
	return &objects.RuntimeModel{ModelID: "gpt-4", Settings: &objects.ModelSettings{ProtocolPools: pools}}
}

func selectorAssociation(channelID int, modelID string, priority int) *objects.ModelAssociation {
	return &objects.ModelAssociation{Type: "channel_model", Priority: priority, ChannelModel: &objects.ChannelModelAssociation{ChannelID: channelID, ModelID: modelID}}
}

func selectorAdapter(format llm.APIFormat, model *objects.RuntimeModel) *objects.RuntimeAdapter {
	return selectorAdapterWithSource(format, "gpt-4", model)
}

func selectorAdapterWithSource(format llm.APIFormat, sourceModelID string, model *objects.RuntimeModel) *objects.RuntimeAdapter {
	return &objects.RuntimeAdapter{ID: 1, Name: "test-adapter", InboundAPIFormat: format.String(), Bindings: map[string]*objects.RuntimeAdapterBinding{sourceModelID: {SourceModelID: sourceModelID, Model: model, Enabled: true}}}
}

func selectorRequest(format llm.APIFormat) *llm.Request {
	return selectorRequestForModel("gpt-4", format)
}

func selectorRequestForModel(model string, format llm.APIFormat) *llm.Request {
	return &llm.Request{Model: model, APIFormat: format}
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
		adapter := selectorAdapter("openai/chat/completions", selectorModel(map[string][]*objects.ModelAssociation{"anthropic": {selectorAssociation(1, "claude", 1)}}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no protocol pool")
		assert.Nil(t, candidates)
	})
	t.Run("no usable target", func(t *testing.T) {
		adapter := selectorAdapter("openai/chat/completions", selectorModel(map[string][]*objects.ModelAssociation{"openai": {}}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no usable target")
		assert.Nil(t, candidates)
	})
	t.Run("openai API format reaches target selection through the openai pool", func(t *testing.T) {
		adapter := selectorAdapterWithSource(llm.APIFormatOpenAIChatCompletion, "DEFAULT", selectorModel(map[string][]*objects.ModelAssociation{
			"openai": {selectorAssociation(1, "gpt-5.6-luna", 1)},
		}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequestForModel("DEFAULT", llm.APIFormatOpenAIChatCompletion))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no usable target")
		assert.NotContains(t, err.Error(), "no protocol pool")
		assert.Nil(t, candidates)
	})
	t.Run("anthropic API format reaches target selection through the anthropic pool", func(t *testing.T) {
		adapter := selectorAdapterWithSource(llm.APIFormatAnthropicMessage, "DEFAULT", selectorModel(map[string][]*objects.ModelAssociation{
			"anthropic": {selectorAssociation(1, "claude-sonnet", 1)},
		}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequestForModel("DEFAULT", llm.APIFormatAnthropicMessage))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no usable target")
		assert.NotContains(t, err.Error(), "no protocol pool")
		assert.Nil(t, candidates)
	})
	t.Run("unknown protocol does not fallback to an openai pool", func(t *testing.T) {
		adapter := selectorAdapterWithSource("gemini/contents", "DEFAULT", selectorModel(map[string][]*objects.ModelAssociation{
			"openai": {selectorAssociation(1, "gpt-5.6-luna", 1)},
		}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequestForModel("DEFAULT", "gemini/contents"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no protocol pool")
		assert.NotContains(t, err.Error(), "no usable target")
		assert.Nil(t, candidates)
	})

	channelSvc.SetEnabledChannelsForTest([]*biz.Channel{
		newSelectorTestBizChannel(1, channel.TypeOpenai, "OpenAI", "https://openai.example", []string{"gpt-4"}, []objects.ChannelEndpoint{selectorEndpoint("openai/chat/completions")}),
		newSelectorTestBizChannel(2, channel.TypeAnthropic, "Anthropic", "https://anthropic.example", []string{"claude"}, []objects.ChannelEndpoint{selectorEndpoint("anthropic/messages")}),
		newSelectorTestBizChannel(3, channel.TypeOpenaiResponses, "OpenAI Responses", "https://responses.example", []string{"gpt-4"}, []objects.ChannelEndpoint{selectorEndpoint(llm.APIFormatOpenAIResponse)}),
	})
	t.Run("openai adapter cannot select anthropic pool", func(t *testing.T) {
		model := selectorModel(map[string][]*objects.ModelAssociation{"anthropic": {selectorAssociation(2, "claude", 1)}})
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, selectorAdapter("openai/chat/completions", model)), selectorRequest("openai/chat/completions"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no protocol pool")
		assert.Nil(t, candidates)
	})
	t.Run("openai responses uses a dedicated protocol pool", func(t *testing.T) {
		model := selectorModel(map[string][]*objects.ModelAssociation{
			"openai_responses": {selectorAssociation(3, "gpt-4", 1)},
		})
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, selectorAdapter(llm.APIFormatOpenAIResponse, model)), selectorRequest(llm.APIFormatOpenAIResponse))
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, 3, candidates[0].Channel.ID)
		assert.Equal(t, string(llm.APIFormatOpenAIResponse), candidates[0].APIFormat)
	})
	t.Run("openai responses does not reuse the chat pool", func(t *testing.T) {
		model := selectorModel(map[string][]*objects.ModelAssociation{
			"openai": {selectorAssociation(1, "gpt-4", 1)},
		})
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, selectorAdapter(llm.APIFormatOpenAIResponse, model)), selectorRequest(llm.APIFormatOpenAIResponse))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no protocol pool")
		assert.Nil(t, candidates)
	})
	t.Run("same model is reused by adapters with isolated pools", func(t *testing.T) {
		model := selectorModel(map[string][]*objects.ModelAssociation{
			"openai":    {selectorAssociation(1, "gpt-4", 1)},
			"anthropic": {selectorAssociation(2, "claude", 2)},
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
	t.Run("source model alias is preserved while selecting the actual target", func(t *testing.T) {
		channelSvc.SetEnabledChannelsForTest([]*biz.Channel{
			newSelectorTestBizChannel(1, channel.TypeOpenai, "OpenAI", "https://openai.example", []string{"gpt-5.6-luna"}, []objects.ChannelEndpoint{selectorEndpoint(llm.APIFormatOpenAIChatCompletion)}),
		})
		adapter := selectorAdapterWithSource(llm.APIFormatOpenAIChatCompletion, "DEFAULT", selectorModel(map[string][]*objects.ModelAssociation{
			"openai": {selectorAssociation(1, "gpt-5.6-luna", 1)},
		}))
		candidates, err := selector.Select(contexts.WithRuntimeAdapter(ctx, adapter), selectorRequestForModel("DEFAULT", llm.APIFormatOpenAIChatCompletion))
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, "DEFAULT", candidates[0].Models[0].RequestModel)
		assert.Equal(t, "gpt-5.6-luna", candidates[0].Models[0].ActualModel)
		assert.Equal(t, string(llm.APIFormatOpenAIChatCompletion), candidates[0].APIFormat)
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

func TestNormalizeProtocolPoolKey(t *testing.T) {
	tests := map[string]string{
		"openai/chat_completions":  "openai",
		"openai/chat/completions":  "openai",
		"openai/responses":         "openai_responses",
		"openai/responses_compact": "openai_responses",
		"anthropic/messages":       "anthropic",
		"gemini/contents":          "gemini/contents",
	}
	for apiFormat, want := range tests {
		t.Run(apiFormat, func(t *testing.T) {
			assert.Equal(t, want, normalizeProtocolPoolKey(apiFormat))
		})
	}
}
