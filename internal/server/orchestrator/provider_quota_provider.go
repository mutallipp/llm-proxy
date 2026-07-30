package orchestrator

import (
	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

// ProviderQuotaStatusProvider provides quota status information for channels.
type ProviderQuotaStatusProvider interface {
	GetQuotaStatus(channelID int) *biz.QuotaChannelStatus
}
