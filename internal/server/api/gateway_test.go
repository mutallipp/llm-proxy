package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutallipp/llm-proxy/internal/authz"
	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/adapter"
	"github.com/mutallipp/llm-proxy/internal/ent/enttest"
	"github.com/mutallipp/llm-proxy/internal/ent/modelgroup"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
)

// setupGatewayTest 创建用于测试的 GatewayHandlers。
func setupGatewayTest(t *testing.T) (*GatewayHandlers, *biz.AdapterService, *ent.Client) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	channelSvc := biz.NewChannelServiceForTest(client)

	adapterSvc := biz.NewAdapterService(biz.AdapterServiceParams{
		Ent:            client,
		ChannelService: channelSvc,
	})

	handlers := NewGatewayHandlers(GatewayHandlersParams{
		AdapterService: adapterSvc,
	})

	return handlers, adapterSvc, client
}

// setupGatewayRouter 创建带有 GatewayHandlers 的测试路由。
func setupGatewayRouter(t *testing.T, handlers *GatewayHandlers) *gin.Engine {
	t.Helper()

	router := gin.New()
	router.GET("/adapters", handlers.ListAdapters)
	router.PUT("/adapters/:name", handlers.UpdateAdapter)
	router.GET("/model-groups", handlers.ListModelGroups)
	router.PUT("/model-groups/:name", handlers.UpdateModelGroup)
	router.GET("/runtime-status", handlers.GetRuntimeStatus)
	router.POST("/refresh", handlers.RefreshGateway)

	return router
}

// TestGatewayHandlers_ListAdapters 测试 ListAdapters。
func TestGatewayHandlers_ListAdapters(t *testing.T) {
	handlers, _, client := setupGatewayTest(t)
	defer client.Close()

	router := setupGatewayRouter(t, handlers)

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("list adapters empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/adapters", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp ListAdaptersResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Empty(t, resp.Adapters)
	})

	t.Run("list adapters with data", func(t *testing.T) {
		// 创建适配器
		_, err := client.Adapter.Create().
			SetName("test-adapter").
			SetInboundAPIFormat("openai/chat/completions").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 创建模型组
		mg, err := client.ModelGroup.Create().
			SetName("test-group").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 创建绑定
		_, err = client.AdapterModelBinding.Create().
			SetAdapterID(1).
			SetModelGroupID(mg.ID).
			SetSourceModelID("gpt-4").
			SetEnabled(true).
			Save(ctx)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/adapters", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp ListAdaptersResponse
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Len(t, resp.Adapters, 1)
		assert.Equal(t, "test-adapter", resp.Adapters[0].Name)
		assert.Len(t, resp.Adapters[0].Bindings, 1)
		assert.Equal(t, "gpt-4", resp.Adapters[0].Bindings[0].SourceModelID)
	})
}

// TestGatewayHandlers_UpdateAdapter 测试 UpdateAdapter。
func TestGatewayHandlers_UpdateAdapter(t *testing.T) {
	handlers, _, client := setupGatewayTest(t)
	defer client.Close()

	router := setupGatewayRouter(t, handlers)

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("update adapter not found", func(t *testing.T) {
		body := `{"display_name": "Updated"}`
		req := httptest.NewRequest(http.MethodPut, "/adapters/non-existent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("update adapter invalid status", func(t *testing.T) {
		// 创建适配器
		_, err := client.Adapter.Create().
			SetName("status-test").
			SetInboundAPIFormat("openai/chat/completions").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		body := `{"status": "invalid"}`
		req := httptest.NewRequest(http.MethodPut, "/adapters/status-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid status")
	})

	t.Run("update adapter invalid binding model_group_id", func(t *testing.T) {
		// 创建适配器
		_, err := client.Adapter.Create().
			SetName("binding-validate-test").
			SetInboundAPIFormat("openai/chat/completions").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 传入无效的 model_group_id
		body := `{"bindings": [{"source_model_id": "gpt-4", "model_group_id": 0}]}`
		req := httptest.NewRequest(http.MethodPut, "/adapters/binding-validate-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("update adapter success", func(t *testing.T) {
		// 创建适配器
		_, err := client.Adapter.Create().
			SetName("update-success-test").
			SetInboundAPIFormat("openai/chat/completions").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		body := `{"display_name": "Updated Display"}`
		req := httptest.NewRequest(http.MethodPut, "/adapters/update-success-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotNil(t, resp["snapshot_version"])
	})
}

// TestGatewayHandlers_UpdateModelGroup 测试 UpdateModelGroup。
func TestGatewayHandlers_UpdateModelGroup(t *testing.T) {
	handlers, _, client := setupGatewayTest(t)
	defer client.Close()

	router := setupGatewayRouter(t, handlers)

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("update model group not found", func(t *testing.T) {
		body := `{"display_name": "Updated"}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/non-existent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("update model group invalid status", func(t *testing.T) {
		_, err := client.ModelGroup.Create().
			SetName("status-test").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		body := `{"status": "invalid"}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/status-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid status")
	})

	t.Run("update model group invalid selection_strategy", func(t *testing.T) {
		_, err := client.ModelGroup.Create().
			SetName("strategy-test").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		body := `{"selection_strategy": "invalid"}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/strategy-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid selection_strategy")
	})

	t.Run("update model group protocol validation", func(t *testing.T) {
		_, err := client.ModelGroup.Create().
			SetName("protocol-test").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 无效的 protocol：inbound_api_format 为空
		body := `{"protocols": [{"inbound_api_format": ""}]}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/protocol-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("update model group target validation - missing channel_id", func(t *testing.T) {
		_, err := client.ModelGroup.Create().
			SetName("target-test").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		body := `{"protocols": [{"inbound_api_format": "openai/chat/completions", "targets": [{"channel_id": 0, "target_model_id": "gpt-4", "outbound_api_format": "openai/chat/completions"}]}]}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/target-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "channel_id")
	})

	t.Run("update model group success", func(t *testing.T) {
		_, err := client.ModelGroup.Create().
			SetName("update-mg-success").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		body := `{"display_name": "Updated MG Display"}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/update-mg-success", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotNil(t, resp["snapshot_version"])
	})
}

// TestGatewayHandlers_ListModelGroups 测试 ListModelGroups。
func TestGatewayHandlers_ListModelGroups(t *testing.T) {
	handlers, _, client := setupGatewayTest(t)
	defer client.Close()

	router := setupGatewayRouter(t, handlers)

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("list model groups empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/model-groups", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp ListModelGroupsResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Empty(t, resp.ModelGroups)
	})

	t.Run("list model groups with protocols", func(t *testing.T) {
		// 创建模型组
		mg, err := client.ModelGroup.Create().
			SetName("test-mg").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 创建协议
		_, err = client.ModelGroupProtocol.Create().
			SetModelGroupID(mg.ID).
			SetInboundAPIFormat("openai/chat/completions").
			SetEnabled(true).
			Save(ctx)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/model-groups", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp ListModelGroupsResponse
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Len(t, resp.ModelGroups, 1)
		require.Len(t, resp.ModelGroups[0].Protocols, 1)
	})
}

// TestGatewayHandlers_GetRuntimeStatus 测试 GetRuntimeStatus。
func TestGatewayHandlers_GetRuntimeStatus(t *testing.T) {
	handlers, adapterSvc, client := setupGatewayTest(t)
	defer client.Close()

	router := setupGatewayRouter(t, handlers)

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// 初始状态
	t.Run("initial status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/runtime-status", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, float64(0), resp["current_snapshot_version"])
	})

	// 刷新后状态
	t.Run("after refresh", func(t *testing.T) {
		// 触发刷新
		_, err := adapterSvc.Refresh(ctx)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/runtime-status", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Greater(t, resp["current_snapshot_version"], float64(0))
	})
}

// TestGatewayHandlers_RefreshGateway 测试 RefreshGateway。
func TestGatewayHandlers_RefreshGateway(t *testing.T) {
	handlers, _, client := setupGatewayTest(t)
	defer client.Close()

	router := setupGatewayRouter(t, handlers)

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("refresh success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Greater(t, resp["snapshot_version"], float64(0))
		assert.NotEmpty(t, resp["refreshed_at"])
	})

	t.Run("refresh with invalid data", func(t *testing.T) {
		// 创建空名称的适配器
		_, err := client.Adapter.Create().
			SetName("").
			SetInboundAPIFormat("openai/chat/completions").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		// 刷新会返回错误
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// TestGatewayHandlers_RequestValidation 测试请求校验。
func TestGatewayHandlers_RequestValidation(t *testing.T) {
	handlers, _, client := setupGatewayTest(t)
	defer client.Close()

	router := setupGatewayRouter(t, handlers)

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("invalid json body", func(t *testing.T) {
		body := `{invalid json}`
		req := httptest.NewRequest(http.MethodPut, "/adapters/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing required binding field", func(t *testing.T) {
		// 创建适配器
		_, err := client.Adapter.Create().
			SetName("binding-field-test").
			SetInboundAPIFormat("openai/chat/completions").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 缺少 source_model_id
		body := `{"bindings": [{"model_group_id": 1}]}`
		req := httptest.NewRequest(http.MethodPut, "/adapters/binding-field-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing required protocol field", func(t *testing.T) {
		// 创建模型组
		_, err := client.ModelGroup.Create().
			SetName("protocol-field-test").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 缺少 inbound_api_format
		body := `{"protocols": [{"enabled": true}]}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/protocol-field-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing required target field", func(t *testing.T) {
		// 创建模型组
		_, err := client.ModelGroup.Create().
			SetName("target-field-test").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 缺少 target_model_id
		body := `{"protocols": [{"inbound_api_format": "openai/chat/completions", "targets": [{"channel_id": 1, "outbound_api_format": "openai/chat/completions"}]}]}`
		req := httptest.NewRequest(http.MethodPut, "/model-groups/target-field-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestGatewayHandlers_ErrorPaths 测试错误路径。
func TestGatewayHandlers_ErrorPaths(t *testing.T) {
	handlers, adapterSvc, client := setupGatewayTest(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	t.Run("refresh fails with invalid adapter config", func(t *testing.T) {
		// 创建会触发校验失败的适配器
		_, err := client.Adapter.Create().
			SetName("fail-refresh").
			SetInboundAPIFormat("").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 刷新
		_, err = adapterSvc.Refresh(ctx)
		require.Error(t, err)
	})

	t.Run("update adapter refresh fails", func(t *testing.T) {
		// 创建适配器
		adapterEnt, err := client.Adapter.Create().
			SetName("refresh-fail-test").
			SetInboundAPIFormat("openai/chat/completions").
			SetStatus(adapter.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 创建模型组用于绑定
		_, err = client.ModelGroup.Create().
			SetName("refresh-fail-group").
			SetStatus(modelgroup.StatusEnabled).
			Save(ctx)
		require.NoError(t, err)

		// 创建绑定
		_, err = client.AdapterModelBinding.Create().
			SetAdapterID(adapterEnt.ID).
			SetModelGroupID(2).
			SetSourceModelID("gpt-4").
			SetEnabled(true).
			Save(ctx)
		require.NoError(t, err)

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.PUT("/adapters/:name", handlers.UpdateAdapter)

		body := `{"display_name": "Updated"}`
		req := httptest.NewRequest(http.MethodPut, "/adapters/refresh-fail-test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		// 由于引用了不存在的 model_group_id，刷新应该失败
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
