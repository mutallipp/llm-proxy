package openapi

import (
	"github.com/99designs/gqlgen/graphql"

	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	apiKeyService                *biz.APIKeyService
	apiKeyProfileTemplateService *biz.APIKeyProfileTemplateService
	quotaService                 *biz.QuotaService
}

func NewSchema(
	apiKeyService *biz.APIKeyService,
	apiKeyProfileTemplateService *biz.APIKeyProfileTemplateService,
	quotaService *biz.QuotaService,
) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: &Resolver{
			apiKeyService:                apiKeyService,
			apiKeyProfileTemplateService: apiKeyProfileTemplateService,
			quotaService:                 quotaService,
		},
	})
}
