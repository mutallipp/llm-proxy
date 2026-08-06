package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/ent/requestexecution"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

func TestBackfillRequestContentAvailability(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:availability-backfill?mode=memory&_fk=0")
	defer client.Close()
	ctx := authz.WithTestBypass(context.Background())

	requestWithEvidence := client.Request.Create().
		SetProjectID(1).
		SetModelID("with-evidence").
		SetStatus(request.StatusCompleted).
		SetStream(true).
		SetRequestBody(objects.JSONRawMessage(`{"model":"with-evidence"}`)).
		SetResponseBody(objects.JSONRawMessage(`{"answer":"ok"}`)).
		SetResponseChunks([]objects.JSONRawMessage{objects.JSONRawMessage(`{"delta":"ok"}`)}).
		SaveX(ctx)

	requestWithPlaceholders := client.Request.Create().
		SetProjectID(1).
		SetModelID("placeholders").
		SetStatus(request.StatusCompleted).
		SetStream(true).
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetResponseBody(objects.JSONRawMessage(`[]`)).
		SetResponseChunks([]objects.JSONRawMessage{}).
		SaveX(ctx)

	nonStreamingRequest := client.Request.Create().
		SetProjectID(1).
		SetModelID("non-streaming").
		SetStatus(request.StatusCompleted).
		SetStream(false).
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SaveX(ctx)

	executionWithEvidence := client.RequestExecution.Create().
		SetProjectID(1).
		SetRequestID(requestWithEvidence.ID).
		SetModelID("with-evidence").
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetRequestBody(objects.JSONRawMessage(`{"model":"with-evidence"}`)).
		SetResponseBody(objects.JSONRawMessage(`{"answer":"ok"}`)).
		SetResponseChunks([]objects.JSONRawMessage{objects.JSONRawMessage(`{"delta":"ok"}`)}).
		SaveX(ctx)

	placeholderExecution := client.RequestExecution.Create().
		SetProjectID(1).
		SetRequestID(requestWithPlaceholders.ID).
		SetModelID("placeholders").
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetResponseBody(objects.JSONRawMessage(`null`)).
		SetResponseChunks([]objects.JSONRawMessage{}).
		SaveX(ctx)

	require.NoError(t, backfillRequestContentAvailability(ctx, client))

	migratedRequest := client.Request.GetX(ctx, requestWithEvidence.ID)
	require.Equal(t, request.RequestBodyAvailabilityAvailable, migratedRequest.RequestBodyAvailability)
	require.Equal(t, request.ResponseBodyAvailabilityAvailable, migratedRequest.ResponseBodyAvailability)
	require.Equal(t, request.ResponseChunksAvailabilityAvailable, migratedRequest.ResponseChunksAvailability)

	unchangedRequest := client.Request.GetX(ctx, requestWithPlaceholders.ID)
	require.Equal(t, request.RequestBodyAvailabilityUnknown, unchangedRequest.RequestBodyAvailability)
	require.Equal(t, request.ResponseBodyAvailabilityUnknown, unchangedRequest.ResponseBodyAvailability)
	require.Equal(t, request.ResponseChunksAvailabilityUnknown, unchangedRequest.ResponseChunksAvailability)

	nonStreaming := client.Request.GetX(ctx, nonStreamingRequest.ID)
	require.Equal(t, request.RequestBodyAvailabilityUnknown, nonStreaming.RequestBodyAvailability)
	require.Equal(t, request.ResponseBodyAvailabilityUnknown, nonStreaming.ResponseBodyAvailability)
	require.Equal(t, request.ResponseChunksAvailabilityNotApplicable, nonStreaming.ResponseChunksAvailability)

	migratedExecution := client.RequestExecution.GetX(ctx, executionWithEvidence.ID)
	require.Equal(t, requestexecution.RequestBodyAvailabilityAvailable, migratedExecution.RequestBodyAvailability)
	require.Equal(t, requestexecution.ResponseBodyAvailabilityAvailable, migratedExecution.ResponseBodyAvailability)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityAvailable, migratedExecution.ResponseChunksAvailability)

	unchangedExecution := client.RequestExecution.GetX(ctx, placeholderExecution.ID)
	require.Equal(t, requestexecution.RequestBodyAvailabilityUnknown, unchangedExecution.RequestBodyAvailability)
	require.Equal(t, requestexecution.ResponseBodyAvailabilityUnknown, unchangedExecution.ResponseBodyAvailability)
	require.Equal(t, requestexecution.ResponseChunksAvailabilityUnknown, unchangedExecution.ResponseChunksAvailability)
}
