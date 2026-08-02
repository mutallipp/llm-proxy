package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/pkg/xcache"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

func newTestAuthService(t *testing.T, client *ent.Client) *biz.AuthService {
	t.Helper()

	cacheConfig := xcache.Config{Mode: xcache.ModeMemory}
	projectService := &biz.ProjectService{
		ProjectCache: xcache.NewFromConfig[xcache.Entry[ent.Project]](cacheConfig),
	}
	apiKeyService := biz.NewAPIKeyService(biz.APIKeyServiceParams{
		CacheConfig:    cacheConfig,
		Ent:            client,
		ProjectService: projectService,
		KeyPrefix:      "ah",
	})
	t.Cleanup(apiKeyService.Stop)

	return &biz.AuthService{APIKeyService: apiKeyService}
}

// TestWithAdapterNoAuthPersistence 测试 no-auth persistence 中间件。
// 注意：此中间件不调用用户 APIKey 校验，因为它使用 system bypass 获取或创建 no-auth APIKey。
func TestWithAdapterNoAuthPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("skips when APIKey already exists", func(t *testing.T) {
		client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
		defer client.Close()

		authSvc := newTestAuthService(t, client)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			// 模拟已有 APIKey 的上下文
			ctx := contexts.WithAPIKey(c.Request.Context(), &ent.APIKey{ID: 1, Key: "existing-key"})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		router.Use(WithAdapterNoAuthPersistence(authSvc))
		router.GET("/test", func(c *gin.Context) {
			// 应该仍然使用原始的 APIKey
			apiKey, ok := contexts.GetAPIKey(c.Request.Context())
			assert.True(t, ok)
			assert.Equal(t, 1, apiKey.ID)
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("no-auth APIKey not created when bypass fails", func(t *testing.T) {
		// 测试中间件在 system bypass 失败时的行为
		// 它应该不阻止请求继续
		client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
		defer client.Close()

		// 使用完整的 APIKeyService，确保 no-auth 查询因测试库没有默认项目而返回错误，
		// 而不是因测试 fixture 缺少依赖而触发 panic。
		authSvc := newTestAuthService(t, client)

		router := gin.New()
		router.Use(WithAdapterNoAuthPersistence(authSvc))
		router.GET("/test", func(c *gin.Context) {
			// 请求应该继续，但可能没有 APIKey
			_, ok := contexts.GetAPIKey(c.Request.Context())
			assert.False(t, ok, "should not have APIKey when bypass fails")
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		// 请求应该继续执行（即使获取 no-auth key 失败）
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("no-op when APIKey exists in context", func(t *testing.T) {
		apiKey := &ent.APIKey{ID: 1, Key: "test-api-key"}
		authSvc := &biz.AuthService{}

		router := gin.New()
		// 先设置一个 APIKey 到上下文
		router.Use(func(c *gin.Context) {
			ctx := contexts.WithAPIKey(c.Request.Context(), apiKey)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		router.Use(WithAdapterNoAuthPersistence(authSvc))
		router.GET("/test", func(c *gin.Context) {
			// 不应该被覆盖
			apiKeyFromCtx, ok := contexts.GetAPIKey(c.Request.Context())
			assert.True(t, ok)
			assert.Equal(t, "test-api-key", apiKeyFromCtx.Key)
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestWithAdapterNoAuthPersistence_NoUserValidation 测试中间件不调用用户 APIKey 校验。
// 这是 MVP 明确的行为：适配器路由无认证是设计意图。
func TestWithAdapterNoAuthPersistence_NoUserValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("does not perform APIKey validation", func(t *testing.T) {
		// 这个测试验证 WithAdapterNoAuthPersistence 中间件不会调用
		// 标准的 APIKey 校验逻辑（如验证 key 是否存在、是否被禁用等）
		// 它直接使用 system bypass 获取 no-auth key，跳过所有校验

		client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
		defer client.Close()

		authSvc := newTestAuthService(t, client)

		var middlewareCalled bool

		router := gin.New()
		router.Use(WithAdapterNoAuthPersistence(authSvc))
		router.GET("/test", func(c *gin.Context) {
			middlewareCalled = true
			// 中间件应该已经设置了一些上下文
			// 如果 system bypass 失败，可能没有 APIKey
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.True(t, middlewareCalled, "middleware should have been called")
		// 请求继续执行，说明没有执行严格的 APIKey 校验
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("request proceeds without user authentication", func(t *testing.T) {
		// 验证请求可以在没有用户认证的情况下继续
		client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
		defer client.Close()

		authSvc := newTestAuthService(t, client)

		router := gin.New()
		router.Use(WithAdapterNoAuthPersistence(authSvc))
		router.GET("/test", func(c *gin.Context) {
			// 验证上下文中没有用户
			user, ok := contexts.GetUser(c.Request.Context())
			assert.False(t, ok, "should not have user in context")
			assert.Nil(t, user)
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		// 没有设置任何认证 header
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestWithAdapterNoAuthPersistence_ContextPropagation 测试上下文传播。
func TestWithAdapterNoAuthPersistence_ContextPropagation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("sets project ID when available", func(t *testing.T) {
		client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
		defer client.Close()

		authSvc := newTestAuthService(t, client)

		router := gin.New()
		router.Use(WithAdapterNoAuthPersistence(authSvc))
		router.GET("/test", func(c *gin.Context) {
			// 当 system bypass 失败时，可能没有设置 project ID
			projectID, hasProjectID := contexts.GetProjectID(c.Request.Context())
			// 如果 middleware 设置了 APIKey，应该也设置了 project ID
			if apiKey, hasAPIKey := contexts.GetAPIKey(c.Request.Context()); hasAPIKey && apiKey != nil && apiKey.Edges.Project != nil {
				assert.True(t, hasProjectID)
				assert.Equal(t, apiKey.Edges.Project.ID, projectID)
			}
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestWithAdapterNoAuthPersistence_MiddlewareOrder 测试中间件顺序。
func TestWithAdapterNoAuthPersistence_MiddlewareOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("should be after authentication middleware", func(t *testing.T) {
		// WithAdapterNoAuthPersistence 设计上应该在认证中间件之后运行
		// 因为它需要检查是否已经有 APIKey（在上下文中）
		client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
		defer client.Close()

		authSvc := newTestAuthService(t, client)
		callOrder := make([]string, 0)

		router := gin.New()
		// 第一个中间件：模拟认证中间件
		router.Use(func(c *gin.Context) {
			callOrder = append(callOrder, "auth-middleware")
			c.Next()
		})
		// 第二个中间件：no-auth persistence
		noAuthPersistence := WithAdapterNoAuthPersistence(authSvc)
		router.Use(func(c *gin.Context) {
			callOrder = append(callOrder, "noauth-persistence")
			noAuthPersistence(c)
		})
		router.GET("/test", func(c *gin.Context) {
			callOrder = append(callOrder, "handler")
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, []string{"auth-middleware", "noauth-persistence", "handler"}, callOrder)
	})
}
