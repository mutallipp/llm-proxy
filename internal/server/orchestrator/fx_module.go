package orchestrator

import (
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"go.uber.org/fx"
)

var Module = fx.Module("orchestrator",
	fx.Provide(NewDefaultSelector),
	fx.Provide(NewAdapterCandidateSelector),
	fx.Provide(NewCandidateSelectorDiagnostics),
	fx.Provide(NewChannelLimiterManager),
	fx.Provide(func(svc *biz.ProviderQuotaService) ProviderQuotaStatusProvider { return svc }),
)
