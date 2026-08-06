package gql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/ent/requestexecution"
	"github.com/mutallipp/llm-proxy/internal/pkg/chunkbuffer"
	"github.com/mutallipp/llm-proxy/internal/pkg/xcache"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/llm/httpclient"
)

func TestRequestResponseChunksAvailabilitySeparatesLivePreviewFromPersistence(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:graphql-availability?mode=memory&_fk=0")
	defer client.Close()

	registry := biz.NewLiveStreamRegistry()
	service := biz.NewRequestService(client, xcache.Config{Mode: xcache.ModeMemory}, nil, nil, nil, registry)
	resolver := &requestResolver{Resolver: &Resolver{requestService: service}}
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	req := client.Request.Create().
		SetProjectID(1).
		SetModelID("live-model").
		SetStatus(request.StatusProcessing).
		SetStream(true).
		SetRequestBody([]byte(`{"model":"live-model"}`)).
		SetResponseChunksAvailability(request.ResponseChunksAvailabilityUnavailable).
		SaveX(ctx)

	buffer := chunkbuffer.New()
	buffer.Append(&httpclient.StreamEvent{Type: "message", Data: []byte(`{"delta":"live"}`)})
	registry.RegisterRequest(req.ID, buffer)

	availability, err := resolver.ResponseChunksAvailability(ctx, req)
	require.NoError(t, err)
	require.Equal(t, request.ResponseChunksAvailabilityAvailable, availability)
	persistedAvailability, err := resolver.ResponseChunksPersistedAvailability(ctx, req)
	require.NoError(t, err)
	require.Equal(t, request.ResponseChunksAvailabilityUnavailable, persistedAvailability)
	live, err := resolver.ResponseChunksLive(ctx, req)
	require.NoError(t, err)
	require.True(t, live)
	require.Equal(t, request.ResponseChunksAvailabilityUnavailable, client.Request.GetX(ctx, req.ID).ResponseChunksAvailability)

	registry.UnregisterRequest(req.ID)
	availability, err = resolver.ResponseChunksAvailability(ctx, req)
	require.NoError(t, err)
	require.Equal(t, request.ResponseChunksAvailabilityUnavailable, availability)
	live, err = resolver.ResponseChunksLive(ctx, req)
	require.NoError(t, err)
	require.False(t, live)

	exec := client.RequestExecution.Create().
		SetProjectID(1).
		SetRequestID(req.ID).
		SetModelID("live-model").
		SetStatus(requestexecution.StatusProcessing).
		SetStream(true).
		SetRequestBody([]byte(`{"model":"live-model"}`)).
		SetResponseChunksAvailability(requestexecution.ResponseChunksAvailabilityUnavailable).
		SaveX(ctx)

	executionResolver := &requestExecutionResolver{Resolver: &Resolver{requestService: service}}
	executionBuffer := chunkbuffer.New()
	executionBuffer.Append(&httpclient.StreamEvent{Type: "message", Data: []byte(`{"delta":"live"}`)})
	registry.RegisterExecution(exec.ID, executionBuffer)

	executionAvailability, err := executionResolver.ResponseChunksAvailability(ctx, exec)
	require.NoError(t, err)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityAvailable, executionAvailability)
	executionPersistedAvailability, err := executionResolver.ResponseChunksPersistedAvailability(ctx, exec)
	require.NoError(t, err)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityUnavailable, executionPersistedAvailability)
	executionLive, err := executionResolver.ResponseChunksLive(ctx, exec)
	require.NoError(t, err)
	require.True(t, executionLive)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityUnavailable, client.RequestExecution.GetX(ctx, exec.ID).ResponseChunksAvailability)
}
