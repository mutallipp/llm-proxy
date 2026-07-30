package orchestrator

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/llm"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
	"github.com/mutallipp/llm-proxy/llm/pipeline"
	"github.com/mutallipp/llm-proxy/llm/pipeline/stream"
	"github.com/mutallipp/llm-proxy/llm/transformer/openai"
)

func TestChatCompletionOrchestrator_Process_MinuteQuotaExceeded(t *testing.T) {
	ctx := context.Background()
	ctx = authz.WithTestBypass(ctx)

	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx = ent.NewContext(ctx, client)

	project := createTestProject(t, ctx, client)
	ch := createTestChannel(t, ctx, client)
	channelService, requestService, systemService, usageLogService := setupTestServices(t, client)
	quotaService := biz.NewQuotaService(client, systemService)

	user, err := client.User.Create().
		SetEmail("quota-test@example.com").
		SetPassword("password").
		Save(ctx)
	require.NoError(t, err)

	apiKey, err := client.APIKey.Create().
		SetName("Quota Test API Key").
		SetKey("ah-quota-test-key").
		SetProjectID(project.ID).
		SetUserID(user.ID).
		SetProfiles(&objects.APIKeyProfiles{
			ActiveProfile: "default",
			Profiles: []objects.APIKeyProfile{
				{
					Name:          "default",
					ModelMappings: []objects.ModelMapping{},
					Quota: &objects.APIKeyQuota{
						Requests: lo.ToPtr(int64(1)),
						Period: objects.APIKeyQuotaPeriod{
							Type: objects.APIKeyQuotaPeriodTypePastDuration,
							PastDuration: &objects.APIKeyQuotaPastDuration{
								Value: 1,
								Unit:  objects.APIKeyQuotaPastDurationUnitMinute,
							},
						},
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	mockResp := buildMockOpenAIResponse("chatcmpl-quota-1", "gpt-4", "ok", 10, 20)
	executor := &mockExecutor{
		response: &httpclient.Response{
			StatusCode: 200,
			Body:       mockResp,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
		},
	}

	outbound, err := openai.NewOutboundTransformer(ch.BaseURL, ch.Credentials.APIKey)
	require.NoError(t, err)

	bizChannel := &biz.Channel{
		Channel:  ch,
		Outbound: outbound,
	}

	channelSelector := &staticChannelSelector{candidates: channelsToTestCandidates([]*biz.Channel{bizChannel}, "gpt-4")}

	orchestrator := &ChatCompletionOrchestrator{
		channelSelector:   channelSelector,
		Inbound:           openai.NewInboundTransformer(),
		RequestService:    requestService,
		ChannelService:    channelService,
		PromptProvider:    &stubPromptProvider{},
		SystemService:     systemService,
		UsageLogService:   usageLogService,
		QuotaService:      quotaService,
		PipelineFactory:   pipeline.NewFactory(executor),
		ModelMapper:       NewModelMapper(),
		channelLimiterManager:      NewChannelLimiterManager(),
		Middlewares: []pipeline.Middleware{
			stream.EnsureUsage(),
		},
	}

	ctx = contexts.WithProjectID(ctx, project.ID)
	ctx = contexts.WithAPIKey(ctx, apiKey)

	httpRequest := buildTestRequest("gpt-4", "Hello!", false)

	_, err = orchestrator.Process(ctx, httpRequest)
	require.NoError(t, err)

	_, err = orchestrator.Process(ctx, httpRequest)
	require.Error(t, err)

	var respErr *llm.ResponseError
	require.ErrorAs(t, err, &respErr)
	require.Equal(t, http.StatusForbidden, respErr.StatusCode)
	require.Equal(t, "quota_exceeded", respErr.Detail.Code)
	require.Equal(t, "quota_exceeded_error", respErr.Detail.Type)
}
