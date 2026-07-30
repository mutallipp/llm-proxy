package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"

	"github.com/mutallipp/llm-proxy/internal/contexts"
	"github.com/mutallipp/llm-proxy/internal/objects"
	"github.com/mutallipp/llm-proxy/internal/server/biz"
	"github.com/mutallipp/llm-proxy/internal/server/orchestrator"
	"github.com/mutallipp/llm-proxy/llm"
)

type AdapterHandlersParams struct {
	fx.In

	AdapterService  *biz.AdapterService
	AdapterSelector *orchestrator.AdapterCandidateSelector
	OpenAI          *OpenAIHandlers
	Anthropic       *AnthropicHandlers
}

// AdapterHandlers 是动态适配器路由的协议门面。
// 请求解析、编排和流式响应均复用现有协议 handler，仅替换候选选择器。
type AdapterHandlers struct {
	AdapterService *biz.AdapterService

	ChatCompletionHandlers     *ChatCompletionHandlers
	ResponseCompletionHandlers *ChatCompletionHandlers
	MessagesHandlers           *ChatCompletionHandlers
}

func NewAdapterHandlers(params AdapterHandlersParams) *AdapterHandlers {
	return &AdapterHandlers{
		AdapterService:             params.AdapterService,
		ChatCompletionHandlers:     withAdapterSelector(params.OpenAI.ChatCompletionHandlers, params.AdapterSelector),
		ResponseCompletionHandlers: withAdapterSelector(params.OpenAI.ResponseCompletionHandlers, params.AdapterSelector),
		MessagesHandlers:           withAdapterSelector(params.Anthropic.ChatCompletionHandlers, params.AdapterSelector),
	}
}

// withAdapterSelector 保留现有流式写出器，并为 handler 副本绑定适配器候选选择器。
func withAdapterSelector(base *ChatCompletionHandlers, selector orchestrator.CandidateSelector) *ChatCompletionHandlers {
	handlers := base.WithStreamWriter(base.StreamWriter)
	handlers.ChatCompletionOrchestrator = base.ChatCompletionOrchestrator.WithChannelSelector(selector)

	return handlers
}

// ChatCompletions 处理 Adapter 的 OpenAI Chat Completions 入站协议。
func (handlers *AdapterHandlers) ChatCompletions(c *gin.Context) {
	if !handlers.requireFormat(c, llm.APIFormatOpenAIChatCompletion) {
		return
	}

	handlers.ChatCompletionHandlers.ChatCompletion(c)
}

// Responses 处理 Adapter 的 OpenAI Responses 入站协议。
func (handlers *AdapterHandlers) Responses(c *gin.Context) {
	if !handlers.requireFormat(c, llm.APIFormatOpenAIResponse) {
		return
	}

	handlers.ResponseCompletionHandlers.ChatCompletion(c)
}

// Messages 处理 Adapter 的 Anthropic Messages 入站协议。
func (handlers *AdapterHandlers) Messages(c *gin.Context) {
	if !handlers.requireFormat(c, llm.APIFormatAnthropicMessage) {
		return
	}

	handlers.MessagesHandlers.ChatCompletion(c)
}

// ListModels 只返回当前 Adapter 绑定的逻辑模型 ID。
func (handlers *AdapterHandlers) ListModels(c *gin.Context) {
	adapter, ok := handlers.resolveAdapter(c)
	if !ok {
		return
	}
	if !handlers.supportsModelAPI(adapter) {
		JSONError(c, http.StatusNotFound, errors.New("adapter operation not found"))
		return
	}

	models, err := handlers.AdapterService.ListModels(c.Request.Context(), adapter.Name)
	if err != nil {
		handlers.writeAdapterError(c, err)
		return
	}

	switch llm.APIFormat(adapter.InboundAPIFormat) {
	case llm.APIFormatAnthropicMessage:
		data := make([]adapterAnthropicModel, 0, len(models))
		for _, modelID := range models {
			data = append(data, adapterAnthropicModel{ID: modelID, Type: "model"})
		}
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": data, "has_more": false})
	default:
		data := make([]adapterOpenAIModel, 0, len(models))
		for _, modelID := range models {
			data = append(data, adapterOpenAIModel{ID: modelID, Object: "model"})
		}
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
	}
}

// RetrieveModel 返回当前 Adapter 的单个逻辑模型。
func (handlers *AdapterHandlers) RetrieveModel(c *gin.Context) {
	adapter, ok := handlers.resolveAdapter(c)
	if !ok {
		return
	}
	if !handlers.supportsModelAPI(adapter) {
		JSONError(c, http.StatusNotFound, errors.New("adapter operation not found"))
		return
	}

	modelID := strings.TrimPrefix(c.Param("model"), "/")
	models, err := handlers.AdapterService.ListModels(c.Request.Context(), adapter.Name)
	if err != nil {
		handlers.writeAdapterError(c, err)
		return
	}

	for _, availableModel := range models {
		if availableModel != modelID {
			continue
		}

		if llm.APIFormat(adapter.InboundAPIFormat) == llm.APIFormatAnthropicMessage {
			c.JSON(http.StatusOK, adapterAnthropicModel{ID: modelID, Type: "model"})
		} else {
			c.JSON(http.StatusOK, adapterOpenAIModel{ID: modelID, Object: "model"})
		}

		return
	}

	JSONError(c, http.StatusNotFound, errors.New("model not found"))
}

type adapterOpenAIModel struct {
	ID     string `json:"id"`
	Object string `json:"object"`
}

type adapterAnthropicModel struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

func (handlers *AdapterHandlers) requireFormat(c *gin.Context, expected llm.APIFormat) bool {
	adapter, ok := handlers.resolveAdapter(c)
	if !ok {
		return false
	}
	if adapter.InboundAPIFormat != string(expected) {
		JSONError(c, http.StatusNotFound, errors.New("adapter operation not found"))
		return false
	}

	return true
}

func (handlers *AdapterHandlers) supportsModelAPI(adapter *objects.RuntimeAdapter) bool {
	switch llm.APIFormat(adapter.InboundAPIFormat) {
	case llm.APIFormatOpenAIChatCompletion, llm.APIFormatOpenAIResponse, llm.APIFormatAnthropicMessage:
		return true
	default:
		return false
	}
}

func (handlers *AdapterHandlers) resolveAdapter(c *gin.Context) (*objects.RuntimeAdapter, bool) {
	if adapter, ok := contexts.GetRuntimeAdapter(c.Request.Context()); ok {
		return adapter, true
	}

	adapterName, ok := contexts.GetAdapterName(c.Request.Context())
	if !ok || adapterName == "" {
		adapterName = strings.TrimSpace(c.Param("adapter"))
	}

	adapter, err := handlers.AdapterService.Resolve(c.Request.Context(), adapterName)
	if err != nil {
		JSONError(c, http.StatusNotFound, errors.New("adapter not found"))
		return nil, false
	}

	ctx := contexts.WithAdapterName(c.Request.Context(), adapter.Name)
	ctx = contexts.WithRuntimeAdapter(ctx, adapter)
	c.Request = c.Request.WithContext(ctx)

	return adapter, true
}

func (handlers *AdapterHandlers) writeAdapterError(c *gin.Context, err error) {
	if errors.Is(err, biz.ErrAdapterNotFound) {
		JSONError(c, http.StatusNotFound, errors.New("adapter not found"))
		return
	}

	JSONError(c, http.StatusInternalServerError, err)
}
