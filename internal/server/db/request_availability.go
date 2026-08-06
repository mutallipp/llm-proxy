package db

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/ent/requestexecution"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

// backfillRequestContentAvailability 为新增字段回填迁移前可确认的内容证据。
// 只能把真实内容标记为 available，不能把历史占位符当作保存证据。
func backfillRequestContentAvailability(ctx context.Context, client *ent.Client) error {
	ctx = authz.WithSystemBypass(ctx, "request-content-availability-backfill")

	requests, err := client.Request.Query().Where(
		request.Or(
			request.RequestBodyAvailabilityEQ(request.RequestBodyAvailabilityUnknown),
			request.ResponseBodyAvailabilityEQ(request.ResponseBodyAvailabilityUnknown),
			request.ResponseChunksAvailabilityEQ(request.ResponseChunksAvailabilityUnknown),
		),
	).All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query requests for content availability backfill: %w", err)
	}

	for _, req := range requests {
		update := client.Request.UpdateOneID(req.ID)
		changed := false

		if req.RequestBodyAvailability == request.RequestBodyAvailabilityUnknown && hasStoredJSONEvidence(req.RequestBody) {
			update = update.SetRequestBodyAvailability(request.RequestBodyAvailabilityAvailable)
			changed = true
		}
		if req.ResponseBodyAvailability == request.ResponseBodyAvailabilityUnknown && hasStoredJSONEvidence(req.ResponseBody) {
			update = update.SetResponseBodyAvailability(request.ResponseBodyAvailabilityAvailable)
			changed = true
		}
		if req.ResponseChunksAvailability == request.ResponseChunksAvailabilityUnknown {
			if !req.Stream {
				update = update.SetResponseChunksAvailability(request.ResponseChunksAvailabilityNotApplicable)
				changed = true
			} else if hasStoredChunksEvidence(req.ResponseChunks) {
				update = update.SetResponseChunksAvailability(request.ResponseChunksAvailabilityAvailable)
				changed = true
			}
		}

		if changed {
			if _, err := update.Save(ctx); err != nil {
				return fmt.Errorf("failed to backfill request %d content availability: %w", req.ID, err)
			}
		}
	}

	executions, err := client.RequestExecution.Query().Where(
		requestexecution.Or(
			requestexecution.RequestBodyAvailabilityEQ(requestexecution.RequestBodyAvailabilityUnknown),
			requestexecution.ResponseBodyAvailabilityEQ(requestexecution.ResponseBodyAvailabilityUnknown),
			requestexecution.ResponseChunksAvailabilityEQ(requestexecution.ResponseChunksAvailabilityUnknown),
		),
	).All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query request executions for content availability backfill: %w", err)
	}

	for _, execution := range executions {
		update := client.RequestExecution.UpdateOneID(execution.ID)
		changed := false

		if execution.RequestBodyAvailability == requestexecution.RequestBodyAvailabilityUnknown && hasStoredJSONEvidence(execution.RequestBody) {
			update = update.SetRequestBodyAvailability(requestexecution.RequestBodyAvailabilityAvailable)
			changed = true
		}
		if execution.ResponseBodyAvailability == requestexecution.ResponseBodyAvailabilityUnknown && hasStoredJSONEvidence(execution.ResponseBody) {
			update = update.SetResponseBodyAvailability(requestexecution.ResponseBodyAvailabilityAvailable)
			changed = true
		}
		if execution.ResponseChunksAvailability == requestexecution.ResponseChunksAvailabilityUnknown {
			if !execution.Stream {
				update = update.SetResponseChunksAvailability(requestexecution.ResponseChunksAvailabilityNotApplicable)
				changed = true
			} else if hasStoredChunksEvidence(execution.ResponseChunks) {
				update = update.SetResponseChunksAvailability(requestexecution.ResponseChunksAvailabilityAvailable)
				changed = true
			}
		}

		if changed {
			if _, err := update.Save(ctx); err != nil {
				return fmt.Errorf("failed to backfill request execution %d content availability: %w", execution.ID, err)
			}
		}
	}

	return nil
}

func hasStoredJSONEvidence(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}

	switch string(trimmed) {
	case "null", "{}", "[]", `{"message":"invalid text"}`:
		return false
	default:
		return json.Valid(trimmed)
	}
}

func hasStoredChunksEvidence(raw []objects.JSONRawMessage) bool {
	for _, chunk := range raw {
		if hasStoredJSONEvidence(chunk) {
			return true
		}
	}

	return false
}
