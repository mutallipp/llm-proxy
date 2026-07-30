package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent/channel"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/llm"
	"github.com/mutallipp/llm-proxy/llm/transformer/nanogpt"
	"github.com/mutallipp/llm-proxy/llm/transformer/openai/responses"
)

func TestNanogptChannel_TypeNanogpt(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())

	entChannel := client.Channel.Create().
		SetName("NanoGPT Deprecated Channel").
		SetType(channel.TypeNanogpt).
		SetBaseURL("https://api.nanogpt.example.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SaveX(ctx)

	channelSvc := NewChannelServiceForTest(client)

	built, err := channelSvc.buildChannelWithTransformer(entChannel)
	require.NoError(t, err)
	require.NotNil(t, built)
	require.NotNil(t, built.Outbound)

	// nanogpt type uses custom transformer with reasoning fields and XML parsing
	_, ok := built.Outbound.(*nanogpt.OutboundTransformer)
	require.True(t, ok, "TypeNanogpt should create nanogpt.OutboundTransformer")
}

func TestNanogptChannel_CreateResponsesTransformer(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())

	entChannel := client.Channel.Create().
		SetName("NanoGPT Responses Channel").
		SetType(channel.TypeNanogptResponses).
		SetBaseURL("https://api.nanogpt.example.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SaveX(ctx)

	channelSvc := NewChannelServiceForTest(client)

	built, err := channelSvc.buildChannelWithTransformer(entChannel)
	require.NoError(t, err)
	require.NotNil(t, built)
	require.NotNil(t, built.Outbound)

	_, ok := built.Outbound.(*responses.OutboundTransformer)
	require.True(t, ok, "TypeNanogptResponses should create responses.OutboundTransformer")
}

func TestNanogptChannel_VerifyAPIFormat(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())

	channelSvc := NewChannelServiceForTest(client)

	t.Run("TypeNanogptResponses returns OpenAI Responses format", func(t *testing.T) {
		entChannel := client.Channel.Create().
			SetName("NanoGPT Responses").
			SetType(channel.TypeNanogptResponses).
			SetBaseURL("https://api.nanogpt.example.com/v1").
			SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
			SetSupportedModels([]string{"gpt-4"}).
			SetDefaultTestModel("gpt-4").
			SaveX(ctx)

		built, err := channelSvc.buildChannelWithTransformer(entChannel)
		require.NoError(t, err)
		require.Equal(t, "openai/responses", string(built.Outbound.APIFormat()))
	})
}

func TestNanogptChannel_BuildChannelWithOutbounds(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())

	entChannel := client.Channel.Create().
		SetName("NanoGPT Multi Endpoint Channel").
		SetType(channel.TypeNanogpt).
		SetBaseURL("https://api.nanogpt.example.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SaveX(ctx)

	channelSvc := NewChannelServiceForTest(client)

	built, err := channelSvc.buildChannelWithOutbounds(entChannel)
	require.NoError(t, err)
	require.NotNil(t, built)
	require.NotNil(t, built.Outbound)

	_, ok := built.Outbound.(*nanogpt.OutboundTransformer)
	require.True(t, ok, "primary outbound should remain nanogpt transformer")

	require.Len(t, built.Outbounds, 9)

	chatOutbound, err := BuildOutboundByAPIFormat(built, llm.APIFormatOpenAIChatCompletion.String())
	require.NoError(t, err)
	require.Same(t, built.Outbound, chatOutbound)

	embeddingOutbound, err := BuildOutboundByAPIFormat(built, llm.APIFormatOpenAIEmbedding.String())
	require.NoError(t, err)
	require.Same(t, built.Outbound, embeddingOutbound)

	imageOutbound, err := BuildOutboundByAPIFormat(built, llm.APIFormatOpenAIImageGeneration.String())
	require.NoError(t, err)
	require.Same(t, built.Outbound, imageOutbound)

	videoOutbound, err := BuildOutboundByAPIFormat(built, llm.APIFormatOpenAIVideo.String())
	require.NoError(t, err)
	require.Same(t, built.Outbound, videoOutbound)
}
