package orchestrator

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/pkg/xjson"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/llm"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

func TestBuildTestRequestUsesConfiguredPrompts(t *testing.T) {
	req := buildChannelTestRequest("test-model", true, "system prompt", "user prompt")

	require.Equal(t, "test-model", req.Model)
	require.Len(t, req.Messages, 2)
	require.Equal(t, "system", req.Messages[0].Role)
	require.Equal(t, "system prompt", *req.Messages[0].Content.Content)
	require.Equal(t, "user", req.Messages[1].Role)
	require.Equal(t, "user prompt", *req.Messages[1].Content.Content)
	require.Equal(t, int64(256), *req.MaxCompletionTokens)
	require.True(t, *req.Stream)
}

func TestNormalizeTestAPIFormatRejectsUnknownProtocol(t *testing.T) {
	format, err := normalizeTestAPIFormat("gemini")
	require.Error(t, err)
	require.Empty(t, format)
}

func TestBuildManagedTestBodyUsesProtocolSpecificFields(t *testing.T) {
	body, err := buildManagedTestBody(llm.APIFormatAnthropicMessage, "test-model", false, "system", "user")
	require.NoError(t, err)
	require.Equal(t, "test-model", gjson.GetBytes(body, "model").String())
	require.Equal(t, "system", gjson.GetBytes(body, "system").String())
	require.Equal(t, "user", gjson.GetBytes(body, "messages.0.content").String())
	require.Equal(t, int64(256), gjson.GetBytes(body, "max_tokens").Int())
}

func TestSelectedModelTargetSelectorValidatesServerTarget(t *testing.T) {
	candidate := &ChannelModelsCandidate{
		Channel:   &biz.Channel{Channel: &ent.Channel{ID: 7, Name: "channel-7"}},
		APIFormat: string(llm.APIFormatOpenAIChatCompletion),
		Models:    []biz.ChannelModelEntry{{RequestModel: "logical", ActualModel: "physical-a"}},
	}
	wrapped := staticCandidateSelector{candidates: []*ChannelModelsCandidate{candidate}}
	selector := selectedModelTargetSelector{
		wrapped:         exactProtocolSelector{wrapped: wrapped, apiFormat: string(llm.APIFormatOpenAIChatCompletion)},
		channelID:       7,
		physicalModelID: "physical-a",
	}

	selected, err := selector.Select(t.Context(), &llm.Request{Model: "logical", APIFormat: llm.APIFormatOpenAIChatCompletion})
	require.NoError(t, err)
	require.Len(t, selected, 1)
	require.Len(t, selected[0].Models, 1)
	require.Equal(t, "physical-a", selected[0].Models[0].ActualModel)

	selector.physicalModelID = "physical-b"
	_, err = selector.Select(t.Context(), &llm.Request{Model: "logical", APIFormat: llm.APIFormatOpenAIChatCompletion})
	require.Error(t, err)
}

type staticCandidateSelector struct {
	candidates []*ChannelModelsCandidate
}

func (s staticCandidateSelector) Select(_ context.Context, _ *llm.Request) ([]*ChannelModelsCandidate, error) {
	return s.candidates, nil
}

func TestManagedTestBodiesAreAcceptedByInboundTransformers(t *testing.T) {
	for _, apiFormat := range []llm.APIFormat{
		llm.APIFormatOpenAIChatCompletion,
		llm.APIFormatOpenAIResponse,
		llm.APIFormatAnthropicMessage,
	} {
		inbound, err := testInboundForAPIFormat(apiFormat)
		require.NoError(t, err)
		body, err := buildManagedTestBody(apiFormat, "test-model", false, "system", "user")
		require.NoError(t, err)
		request, err := inbound.TransformRequest(t.Context(), &httpclient.Request{
			Headers: http.Header{"Content-Type": []string{"application/json"}},
			Body:    body,
		})
		require.NoError(t, err, string(body))
		require.Equal(t, "test-model", request.Model)
		require.NotEmpty(t, request.Messages)
		_, err = xjson.Marshal(request)
		require.NoError(t, err)
	}
}
