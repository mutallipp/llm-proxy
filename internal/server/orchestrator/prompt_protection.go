package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/mutallipp/llm-proxy/internal/log"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/llm"
	"github.com/mutallipp/llm-proxy/llm/pipeline"
	"github.com/mutallipp/llm-proxy/llm/transformer"
)

const promptProtectionRejectedMessage = "request blocked by prompt protection policy"

func protectPrompts(inbound *PersistentInboundTransformer) pipeline.Middleware {
	return pipeline.OnLlmRequest("protect-prompts", func(ctx context.Context, llmRequest *llm.Request) (*llm.Request, error) {
		if inbound.state.PromptProtecter == nil {
			return llmRequest, nil
		}

		protected, err := inbound.state.PromptProtecter.Protect(ctx, llmRequest)
		if err != nil {
			if errors.Is(err, biz.ErrPromptProtectionRejected) {
				return nil, fmt.Errorf("%w: %s", transformer.ErrInvalidRequest, promptProtectionRejectedMessage)
			}

			log.Warn(ctx, "failed to protect prompts", log.Cause(err))

			return llmRequest, nil
		}

		if protected == nil {
			return llmRequest, nil
		}

		return protected, nil
	})
}
