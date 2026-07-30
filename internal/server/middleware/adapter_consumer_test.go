package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/mutallipp/llm-proxy/internal/contexts"
)

// TestWithAdapterConsumerInterceptor_ExtractAPIKey 测试提取 APIKey。
func TestWithAdapterConsumerInterceptor_ExtractAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		headers     map[string]string
		expectedKey string
		expectInCtx bool
	}{
		{
			name:        "extract from Authorization Bearer",
			headers:     map[string]string{"Authorization": "Bearer test-key-bearer"},
			expectedKey: "test-key-bearer",
			expectInCtx: true,
		},
		{
			name:        "extract from Authorization Token",
			headers:     map[string]string{"Authorization": "Token test-key-token"},
			expectedKey: "test-key-token",
			expectInCtx: true,
		},
		{
			name:        "extract from x-api-key",
			headers:     map[string]string{"x-api-key": "test-key-xapi"},
			expectedKey: "test-key-xapi",
			expectInCtx: true,
		},
		{
			name:        "extract from api-key",
			headers:     map[string]string{"api-key": "test-key-apikey"},
			expectedKey: "test-key-apikey",
			expectInCtx: true,
		},
		{
			name:        "Authorization without prefix",
			headers:     map[string]string{"Authorization": "sk-1234567890"},
			expectedKey: "sk-1234567890",
			expectInCtx: true,
		},
		{
			name:        "no credentials",
			headers:     map[string]string{},
			expectedKey: "",
			expectInCtx: false,
		},
		{
			name:        "empty Authorization",
			headers:     map[string]string{"Authorization": ""},
			expectedKey: "",
			expectInCtx: false,
		},
		{
			name:        "priority: Authorization Bearer over x-api-key",
			headers:     map[string]string{"Authorization": "Bearer key1", "x-api-key": "key2"},
			expectedKey: "key1",
			expectInCtx: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(WithAdapterConsumerInterceptor())
			router.GET("/test", func(c *gin.Context) {
				key, ok := contexts.GetAdapterConsumerAPIKey(c.Request.Context())
				if tt.expectInCtx {
					assert.True(t, ok, "expected APIKey in context")
					assert.Equal(t, tt.expectedKey, key, "APIKey mismatch")
				} else {
					assert.False(t, ok, "did not expect APIKey in context")
				}
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tt.headers {
				if v != "" {
					req.Header.Set(k, v)
				}
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestWithAdapterConsumerInterceptor_RemoveOutboundHeaders 测试清理出站 Header。
func TestWithAdapterConsumerInterceptor_RemoveOutboundHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		inboundHeaders  map[string]string
		expectedRemoved []string // 应该被删除的 header
		expectedKept    []string // 应该保留的 header
	}{
		{
			name: "remove Authorization",
			inboundHeaders: map[string]string{
				"Authorization": "Bearer secret-key",
				"Content-Type":  "application/json",
				"X-Request-ID":  "12345",
			},
			expectedRemoved: []string{"Authorization"},
			expectedKept:    []string{"Content-Type", "X-Request-ID"},
		},
		{
			name: "remove x-api-key",
			inboundHeaders: map[string]string{
				"x-api-key":    "secret-key",
				"Content-Type": "application/json",
			},
			expectedRemoved: []string{"x-api-key"},
			expectedKept:    []string{"Content-Type"},
		},
		{
			name: "remove api-key",
			inboundHeaders: map[string]string{
				"api-key":      "secret-key",
				"X-Request-ID": "12345",
			},
			expectedRemoved: []string{"api-key"},
			expectedKept:    []string{"X-Request-ID"},
		},
		{
			name: "case insensitive removal",
			inboundHeaders: map[string]string{
				"AUTHORIZATION": "Bearer secret",
				"X-API-KEY":     "secret",
				"API-KEY":       "secret",
			},
			expectedRemoved: []string{"AUTHORIZATION", "X-API-KEY", "API-KEY"},
			expectedKept:    []string{},
		},
		{
			name: "keep other headers",
			inboundHeaders: map[string]string{
				"Authorization":   "Bearer secret",
				"x-api-key":       "secret",
				"Content-Type":    "application/json",
				"X-Custom-Header": "custom-value",
				"User-Agent":      "TestAgent/1.0",
			},
			expectedRemoved: []string{"Authorization", "x-api-key"},
			expectedKept:    []string{"Content-Type", "X-Custom-Header", "User-Agent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedHeaders http.Header

			router := gin.New()
			router.Use(WithAdapterConsumerInterceptor())
			router.GET("/test", func(c *gin.Context) {
				capturedHeaders = c.Request.Header.Clone()
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tt.inboundHeaders {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)

			// 验证被删除的 header
			for _, h := range tt.expectedRemoved {
				assert.NotContains(t, capturedHeaders.Get(h), "secret", "header %s should be removed", h)
				assert.Empty(t, capturedHeaders.Values(h), "header %s should not exist in outbound request", h)
			}

			// 验证保留的 header
			for _, h := range tt.expectedKept {
				assert.NotEmpty(t, capturedHeaders.Get(h), "header %s should be kept", h)
			}
		})
	}
}

// TestWithAdapterConsumerInterceptor_DefaultPassThrough 测试默认放行。
func TestWithAdapterConsumerInterceptor_DefaultPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("pass through without credentials", func(t *testing.T) {
		router := gin.New()
		router.Use(WithAdapterConsumerInterceptor())
		router.GET("/test", func(c *gin.Context) {
			// 没有凭证也应该继续处理
			key, ok := contexts.GetAdapterConsumerAPIKey(c.Request.Context())
			assert.False(t, ok, "should not have APIKey in context")
			assert.Empty(t, key)
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("pass through with invalid credentials", func(t *testing.T) {
		router := gin.New()
		router.Use(WithAdapterConsumerInterceptor())
		router.GET("/test", func(c *gin.Context) {
			// 即使凭证无效（空字符串），也应该继续处理
			key, ok := contexts.GetAdapterConsumerAPIKey(c.Request.Context())
			assert.False(t, ok, "should not have APIKey in context for empty value")
			assert.Empty(t, key)
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestRemoveAdapterConsumerCredentials 测试辅助函数。
func TestRemoveAdapterConsumerCredentials(t *testing.T) {
	tests := []struct {
		name         string
		inputHeaders map[string]string
		expectEmpty  []string
		expectPres   map[string]string
	}{
		{
			name: "remove all credential headers",
			inputHeaders: map[string]string{
				"Authorization": "Bearer secret",
				"x-api-key":     "secret",
				"api-key":       "secret",
			},
			expectEmpty: []string{"Authorization", "x-api-key", "api-key"},
			expectPres:  map[string]string{},
		},
		{
			name: "preserve normal headers",
			inputHeaders: map[string]string{
				"Content-Type":  "application/json",
				"X-Request-ID":  "12345",
				"Authorization": "Bearer secret",
			},
			expectEmpty: []string{"Authorization"},
			expectPres: map[string]string{
				"Content-Type": "application/json",
				"X-Request-ID": "12345",
			},
		},
		{
			name: "case insensitive",
			inputHeaders: map[string]string{
				"authorization": "Bearer secret",
				"AUTHORIZATION": "Bearer secret2",
			},
			expectEmpty: []string{"authorization", "AUTHORIZATION"},
			expectPres:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			for k, v := range tt.inputHeaders {
				headers.Set(k, v)
			}

			result := removeAdapterConsumerCredentials(headers)

			// 验证应该为空的 header
			for _, h := range tt.expectEmpty {
				assert.Empty(t, result.Values(h), "header %s should be empty", h)
			}

			// 验证应该保留的 header
			for h, v := range tt.expectPres {
				assert.Equal(t, v, result.Get(h), "header %s should be preserved", h)
			}
		})
	}
}

// TestWithAdapterConsumerInterceptor_PreservesOriginalRequest 测试原始请求完整性。
func TestWithAdapterConsumerInterceptor_PreservesOriginalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("original request body is preserved", func(t *testing.T) {
		var body []byte

		router := gin.New()
		router.Use(WithAdapterConsumerInterceptor())
		router.POST("/test", func(c *gin.Context) {
			body, _ = c.GetRawData()
			c.Status(http.StatusOK)
		})

		reqBody := `{"key": "value", "model": "gpt-4"}`
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer secret")
		req.Body = &mockReadCloser{data: []byte(reqBody)}
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, reqBody, string(body))
	})

	t.Run("URL and method are preserved", func(t *testing.T) {
		var method, url string

		router := gin.New()
		router.Use(WithAdapterConsumerInterceptor())
		router.POST("/custom/path", func(c *gin.Context) {
			method = c.Request.Method
			url = c.Request.URL.Path
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodPost, "/custom/path", nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, http.MethodPost, method)
		assert.Equal(t, "/custom/path", url)
	})
}

// mockReadCloser 用于测试请求体。
type mockReadCloser struct {
	data []byte
	pos  int
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, nil
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}
