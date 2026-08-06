package biz

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/ent/requestexecution"
	"github.com/mutallipp/llm-proxy/internal/pkg/chunkbuffer"
	"github.com/mutallipp/llm-proxy/internal/pkg/xcache"
	"github.com/mutallipp/llm-proxy/llm"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

func TestRequestContentAvailabilityTracksStoredAndUnavailableContent(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	defer client.Close()

	cacheConfig := xcache.Config{Mode: xcache.ModeMemory}
	systemService := NewSystemService(SystemServiceParams{Ent: client, CacheConfig: cacheConfig})
	dataStorageService := NewDataStorageService(DataStorageServiceParams{
		SystemService: systemService,
		CacheConfig:   cacheConfig,
		Client:        client,
	})
	requestService := NewRequestService(client, cacheConfig, systemService, nil, dataStorageService, nil)
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	err := systemService.SetStoragePolicy(ctx, &StoragePolicy{
		StoreRequestBody:  false,
		StoreResponseBody: true,
		StoreChunks:       false,
	})
	require.NoError(t, err)

	input := &llm.Request{Model: "logical", Stream: lo.ToPtr(false)}
	rawRequest := &httpclient.Request{
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"model":"logical"}`),
	}
	created, err := requestService.CreateRequest(ctx, input, rawRequest, llm.APIFormatOpenAIChatCompletion)
	require.NoError(t, err)
	require.Equal(t, request.RequestBodyAvailabilityUnavailable, created.RequestBodyAvailability)
	require.Equal(t, request.ResponseBodyAvailabilityUnavailable, created.ResponseBodyAvailability)
	require.Equal(t, request.ResponseChunksAvailabilityNotApplicable, created.ResponseChunksAvailability)

	err = requestService.UpdateRequestCompleted(ctx, created.ID, "provider-id", map[string]any{"answer": "ok"}, nil)
	require.NoError(t, err)
	updated := client.Request.GetX(ctx, created.ID)
	require.Equal(t, request.ResponseBodyAvailabilityAvailable, updated.ResponseBodyAvailability)

	execution, err := requestService.CreateRequestExecution(
		ctx,
		&Channel{Channel: &ent.Channel{ID: 1}},
		"physical",
		created,
		*rawRequest,
		llm.APIFormatOpenAIChatCompletion,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, requestexecution.RequestBodyAvailabilityUnavailable, execution.RequestBodyAvailability)
	require.Equal(t, requestexecution.ResponseBodyAvailabilityUnavailable, execution.ResponseBodyAvailability)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityNotApplicable, execution.ResponseChunksAvailability)

	err = requestService.UpdateRequestExecutionCompleted(ctx, execution.ID, "provider-exec-id", map[string]any{"answer": "ok"}, nil)
	require.NoError(t, err)
	updatedExecution := client.RequestExecution.GetX(ctx, execution.ID)
	require.Equal(t, requestexecution.ResponseBodyAvailabilityAvailable, updatedExecution.ResponseBodyAvailability)
}

func TestRequestContentAvailabilityDistinguishesEmptyStoredChunks(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	defer client.Close()

	cacheConfig := xcache.Config{Mode: xcache.ModeMemory}
	systemService := NewSystemService(SystemServiceParams{Ent: client, CacheConfig: cacheConfig})
	dataStorageService := NewDataStorageService(DataStorageServiceParams{
		SystemService: systemService,
		CacheConfig:   cacheConfig,
		Client:        client,
	})
	liveRegistry := NewLiveStreamRegistry()
	requestService := NewRequestService(client, cacheConfig, systemService, nil, dataStorageService, liveRegistry)
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	err := systemService.SetStoragePolicy(ctx, &StoragePolicy{
		StoreRequestBody:  true,
		StoreResponseBody: true,
		StoreChunks:       true,
	})
	require.NoError(t, err)

	input := &llm.Request{Model: "logical", Stream: lo.ToPtr(true)}
	rawRequest := &httpclient.Request{Body: []byte(`{"model":"logical","stream":true}`)}
	created, err := requestService.CreateRequest(ctx, input, rawRequest, llm.APIFormatOpenAIChatCompletion)
	require.NoError(t, err)
	require.Equal(t, request.ResponseChunksAvailabilityUnavailable, created.ResponseChunksAvailability)

	liveBuffer := chunkbuffer.New()
	liveRegistry.RegisterRequest(created.ID, liveBuffer)
	liveBuffer.Append(&httpclient.StreamEvent{Type: "message", Data: []byte(`{"ok":true}`)})
	require.True(t, requestService.IsRequestResponseChunksLive(created))
	liveChunks, err := requestService.LoadResponseChunks(ctx, created)
	require.NoError(t, err)
	require.Len(t, liveChunks, 1)
	require.Equal(t, request.ResponseChunksAvailabilityUnavailable, client.Request.GetX(ctx, created.ID).ResponseChunksAvailability)
	liveRegistry.UnregisterRequest(created.ID)

	err = requestService.SaveRequestChunks(ctx, created.ID, []*httpclient.StreamEvent{{Type: "message", Data: []byte(`{"ok":true}`)}})
	require.NoError(t, err)
	updated := client.Request.GetX(ctx, created.ID)
	require.Equal(t, request.ResponseChunksAvailabilityAvailable, updated.ResponseChunksAvailability)
	require.Len(t, updated.ResponseChunks, 1)

	emptyCreated, err := requestService.CreateRequest(ctx, input, rawRequest, llm.APIFormatOpenAIChatCompletion)
	require.NoError(t, err)
	err = requestService.SaveRequestChunks(ctx, emptyCreated.ID, []*httpclient.StreamEvent{{Data: llm.DoneStreamEvent.Data}})
	require.NoError(t, err)
	emptyUpdated := client.Request.GetX(ctx, emptyCreated.ID)
	require.Equal(t, request.ResponseChunksAvailabilityAvailable, emptyUpdated.ResponseChunksAvailability)
	require.Empty(t, emptyUpdated.ResponseChunks)

	execution, err := requestService.CreateRequestExecution(
		ctx,
		&Channel{Channel: &ent.Channel{ID: 1}},
		"physical",
		created,
		*rawRequest,
		llm.APIFormatOpenAIChatCompletion,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityUnavailable, execution.ResponseChunksAvailability)

	executionBuffer := chunkbuffer.New()
	liveRegistry.RegisterExecution(execution.ID, executionBuffer)
	executionBuffer.Append(&httpclient.StreamEvent{Type: "message", Data: []byte(`{"ok":true}`)})
	require.True(t, requestService.IsRequestExecutionResponseChunksLive(execution))
	liveExecutionChunks, err := requestService.LoadRequestExecutionResponseChunks(ctx, execution)
	require.NoError(t, err)
	require.Len(t, liveExecutionChunks, 1)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityUnavailable, client.RequestExecution.GetX(ctx, execution.ID).ResponseChunksAvailability)
	liveRegistry.UnregisterExecution(execution.ID)

	err = requestService.SaveRequestExecutionChunks(ctx, execution.ID, []*httpclient.StreamEvent{{Type: "message", Data: []byte(`{"ok":true}`)}})
	require.NoError(t, err)
	updatedExecution := client.RequestExecution.GetX(ctx, execution.ID)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityAvailable, updatedExecution.ResponseChunksAvailability)
	require.Len(t, updatedExecution.ResponseChunks, 1)
}
