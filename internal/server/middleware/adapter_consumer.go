package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/llm/transformer/shared"
)

var adapterConsumerAPIKeyConfig = &APIKeyConfig{
	Headers:         []string{"Authorization", "x-api-key", "api-key"},
	RequireBearer:   false,
	AllowedPrefixes: []string{"Bearer ", "Token ", "Api-Key ", "API-Key "},
}

// WithAdapterConsumerInterceptor 提取适配器消费端凭证，并移除其出站传播路径。
// 一期只将凭证放入请求上下文，不做校验、不落库、不要求存在，也不转发给渠道。
func WithAdapterConsumerInterceptor() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if key, err := ExtractAPIKeyFromRequest(c.Request, adapterConsumerAPIKeyConfig); err == nil && key != "" {
			ctx = contexts.WithAdapterConsumerAPIKey(ctx, key)
		}

		request := c.Request.Clone(ctx)
		request.Header = removeAdapterConsumerCredentials(request.Header)
		c.Request = request
		c.Next()
	}
}

// WithAdapterRoute 将动态路由绑定到当前启用的适配器快照。
// 适配器不存在或已被禁用时统一返回 404，不暴露数据库配置详情。
func WithAdapterRoute(adapterService *biz.AdapterService) gin.HandlerFunc {
	return func(c *gin.Context) {
		adapter, err := adapterService.Resolve(c.Request.Context(), strings.TrimSpace(c.Param("adapter")))
		if err != nil {
			AbortWithError(c, http.StatusNotFound, errors.New("adapter not found"))
			return
		}

		ctx := contexts.WithAdapterName(c.Request.Context(), adapter.Name)
		ctx = contexts.WithRuntimeAdapter(ctx, adapter)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// WithAdapterNoAuthPersistence 确保无凭证的适配器请求能正确持久化。
// 当 adapter consumer 凭证提取未找到 APIKey 时，尝试获取或创建 no-auth APIKey，
// 并将其关联到请求上下文，使 RequestService.CreateRequest 等操作能正常执行。
// 此中间件不执行认证校验，适配器路由无认证是 MVP 明确的行为。
func WithAdapterNoAuthPersistence(authService *biz.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 如果已有 APIKey（例如通过 adapter consumer 凭证匹配到），跳过
		if _, ok := contexts.GetAPIKey(c.Request.Context()); ok {
			c.Next()
			return
		}

		// 通过 system bypass 获取或创建 no-auth APIKey（复用 EnsureNoAuthAPIKey 机制）
		apiKey, err := authz.RunWithSystemBypass(c.Request.Context(), "adapter-noauth",
			func(bypassCtx context.Context) (*ent.APIKey, error) {
				return authService.APIKeyService.EnsureNoAuthAPIKey(bypassCtx)
			})
		if err != nil {
			// 无法获取 no-auth key 时不阻止请求，继续让请求通过（仍然持久化，只是无 project 关联）
			c.Next()
			return
		}

		// 设置请求上下文，使 RequestService.CreateRequest 等能正确关联
		ctx := c.Request.Context()
		ctx = contexts.WithAPIKey(ctx, apiKey)
		if apiKey.Edges.Project != nil {
			ctx = contexts.WithProjectID(ctx, apiKey.Edges.Project.ID)
		}
		ctx = shared.WithSessionScope(ctx, "api_key:"+strconv.Itoa(apiKey.ID)+":project:"+strconv.Itoa(apiKey.Edges.Project.ID))
		ctx, _ = withAPIKeyPrincipal(ctx, apiKey)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func removeAdapterConsumerCredentials(header http.Header) http.Header {
	result := header.Clone()
	for name := range result {
		switch strings.ToLower(name) {
		case "authorization", "x-api-key", "api-key":
			delete(result, name)
		}
	}

	return result
}
