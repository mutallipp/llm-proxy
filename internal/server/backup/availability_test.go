package backup

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

func TestNormalizeAvailabilityUsesOnlyLegacyContentEvidence(t *testing.T) {
	require.Equal(t,
		request.RequestBodyAvailabilityAvailable,
		normalizeRequestBodyAvailability(request.RequestBodyAvailabilityUnknown, objects.JSONRawMessage(`{"model":"gpt"}`)),
	)
	require.Equal(t,
		request.RequestBodyAvailabilityUnknown,
		normalizeRequestBodyAvailability(request.RequestBodyAvailabilityUnknown, objects.JSONRawMessage(`{}`)),
	)
	require.Equal(t,
		request.ResponseBodyAvailabilityAvailable,
		normalizeResponseBodyAvailability(request.ResponseBodyAvailabilityUnknown, objects.JSONRawMessage(`{"answer":"ok"}`)),
	)
	require.Equal(t,
		request.ResponseBodyAvailabilityUnknown,
		normalizeResponseBodyAvailability(request.ResponseBodyAvailabilityUnknown, objects.JSONRawMessage(`[]`)),
	)
	require.Equal(t,
		request.ResponseChunksAvailabilityAvailable,
		normalizeResponseChunksAvailability(request.ResponseChunksAvailabilityUnknown, []objects.JSONRawMessage{objects.JSONRawMessage(`{"delta":"ok"}`)}, true),
	)
	require.Equal(t,
		request.ResponseChunksAvailabilityUnknown,
		normalizeResponseChunksAvailability(request.ResponseChunksAvailabilityUnknown, nil, true),
	)
	require.Equal(t,
		request.ResponseChunksAvailabilityUnknown,
		normalizeResponseChunksAvailability(request.ResponseChunksAvailabilityUnknown, []objects.JSONRawMessage{objects.JSONRawMessage(`{}`)}, true),
	)
	require.Equal(t,
		request.ResponseChunksAvailabilityNotApplicable,
		normalizeResponseChunksAvailability(request.ResponseChunksAvailabilityUnknown, nil, false),
	)
}
