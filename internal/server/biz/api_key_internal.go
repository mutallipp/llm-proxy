package biz

import (
	"context"
	"time"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
)

func (s *APIKeyService) onLoadOneKey(ctx context.Context, cacheKey string) (*ent.APIKey, error) {
	bypassCtx := authz.WithSystemBypass(ctx, "apikey-cache-load-one")
	return s.loadAPIKeyByKey(bypassCtx, cacheKey)
}

func (s *APIKeyService) onLoadAPIKeysSince(ctx context.Context, since time.Time) ([]*ent.APIKey, time.Time, error) {
	bypassCtx := authz.WithSystemBypass(ctx, "apikey-cache-load-since")
	return s.loadAPIKeysSince(bypassCtx, since)
}
