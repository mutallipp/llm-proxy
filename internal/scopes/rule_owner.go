package scopes

import (
	"context"

	"github.com/mutallipp/llm-proxy/internal/ent/privacy"
)

// OwnerRule allows owner users to access all functionality.
func OwnerRule() privacy.QueryMutationRule {
	return privacy.ContextQueryMutationRule(func(ctx context.Context) error {
		user, err := getUserFromContext(ctx)
		if err != nil {
			return privacy.Skipf("User not found in context")
		}

		// Owner users have all permissions
		if user.IsOwner {
			return privacy.Allow
		}

		return privacy.Skip
	})
}
