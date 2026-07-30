package orchestrator

import (
	"context"

	"github.com/mutallipp/llm-proxy/llm"
)

type PromptProtecter interface {
	Protect(ctx context.Context, req *llm.Request) (*llm.Request, error)
}
