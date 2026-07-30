package biz

import (
	"context"

	"github.com/mutallipp/llm-proxy/internal/authz"
)

func (svc *ProviderQuotaService) runQuotaCheckScheduled(ctx context.Context) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	ctx = authz.WithSystemBypass(ctx, "provider_quota")
	svc.runQuotaCheck(ctx, false)
}
