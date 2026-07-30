package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
)

// setupAdapterSelectorTest 创建用于测试的 AdapterCandidateSelector。
func setupAdapterSelectorTest(t *testing.T) (*AdapterCandidateSelector, *biz.ChannelService, *ent.Client) {
	t.Helper()

	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	channelSvc := biz.NewChannelServiceForTest(client)
	adapterSvc := biz.NewAdapterService(biz.AdapterServiceParams{
		Ent:            client,
		ChannelService: channelSvc,
	})

	selector := NewAdapterCandidateSelector(adapterSvc, channelSvc)

	return selector, channelSvc, client
}

// newSelectorTestBizChannel 创建用于测试的 biz.Channel。
func newSelectorTestBizChannel(id int, channelType channel.Type, name, baseURL string, supportedModels []string, endpoints []objects.ChannelEndpoint) *biz.Channel {
	return &biz.Channel{
		Channel: &ent.Channel{
			ID:               id,
			Type:             channelType,
			Name:             name,
			BaseURL:          baseURL,
			SupportedModels:  supportedModels,
			DefaultTestModel: supportedModels[0],
			Endpoints:        endpoints,
		},
	}
}

// createSelectorEndpoint 创建渠道端点。
func createSelectorEndpoint(apiFormat, basePath string) objects.ChannelEndpoint {
	return objects.ChannelEndpoint{
		APIFormat: apiFormat,
		Path:      basePath,
	}
}

// TestAdapterCandidateSelector_Select 测试 Select 方法。
func TestAdapterCandidateSelector_Select(t *testing.T) {
	selector, channelSvc, client := setupAdapterSelectorTest(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)

	t.Run("nil request returns error", func(t *testing.T) {
		candidates, err := selector.Select(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, candidates)
		assert.Contains(t, err.Error(), "adapter request is nil")
	})

	t.Run("missing runtime adapter context", func(t *testing.T) {
		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "openai/chat/completions",
		}
		candidates, err := selector.Select(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, candidates)
	})

	t.Run("inbound API format mismatch", func(t *testing.T) {
		// 设置上下文中的适配器
		runtimeAdapter := &objects.RuntimeAdapter{
			ID:               1,
			Name:             "test-adapter",
			InboundAPIFormat: "openai/chat/completions",
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "anthropic/messages", // 不匹配的格式
		}
		candidates, err := selector.Select(ctxWithAdapter, req)
		assert.Error(t, err)
		assert.Nil(t, candidates)
		assert.Contains(t, err.Error(), "inbound api format")
	})

	t.Run("source model not bound", func(t *testing.T) {
		runtimeAdapter := &objects.RuntimeAdapter{
			ID:               1,
			Name:             "test-adapter",
			InboundAPIFormat: "openai/chat/completions",
			Bindings:         map[string]*objects.RuntimeAdapterBinding{},
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "openai/chat/completions",
		}
		candidates, err := selector.Select(ctxWithAdapter, req)
		assert.Error(t, err)
		assert.Nil(t, candidates)
		assert.Contains(t, err.Error(), "not bound to adapter")
	})

	t.Run("binding disabled", func(t *testing.T) {
		runtimeAdapter := &objects.RuntimeAdapter{
			ID:               1,
			Name:             "test-adapter",
			InboundAPIFormat: "openai/chat/completions",
			Bindings: map[string]*objects.RuntimeAdapterBinding{
				"gpt-4": {
					SourceModelID: "gpt-4",
					Enabled:       false,
				},
			},
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "openai/chat/completions",
		}
		candidates, err := selector.Select(ctxWithAdapter, req)
		assert.Error(t, err)
		assert.Nil(t, candidates)
		assert.Contains(t, err.Error(), "not bound to adapter")
	})

	t.Run("model group missing", func(t *testing.T) {
		runtimeAdapter := &objects.RuntimeAdapter{
			ID:               1,
			Name:             "test-adapter",
			InboundAPIFormat: "openai/chat/completions",
			Bindings: map[string]*objects.RuntimeAdapterBinding{
				"gpt-4": {
					SourceModelID: "gpt-4",
					ModelGroup:    nil,
					Enabled:       true,
				},
			},
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "openai/chat/completions",
		}
		candidates, err := selector.Select(ctxWithAdapter, req)
		assert.Error(t, err)
		assert.Nil(t, candidates)
		assert.Contains(t, err.Error(), "has no model group")
	})

	t.Run("no usable target", func(t *testing.T) {
		runtimeAdapter := &objects.RuntimeAdapter{
			ID:               1,
			Name:             "test-adapter",
			InboundAPIFormat: "openai/chat/completions",
			Bindings: map[string]*objects.RuntimeAdapterBinding{
				"gpt-4": {
					SourceModelID: "gpt-4",
					ModelGroup: &objects.RuntimeModelGroup{
						Name: "test-group",
						Protocols: map[string]*objects.RuntimeModelGroupProtocol{
							"openai/chat/completions": {
								InboundAPIFormat: "openai/chat/completions",
								Targets:          []*objects.RuntimeModelGroupTarget{}, // 空目标
							},
						},
					},
					Enabled: true,
				},
			},
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "openai/chat/completions",
		}
		candidates, err := selector.Select(ctxWithAdapter, req)
		assert.Error(t, err)
		assert.Nil(t, candidates)
		assert.Contains(t, err.Error(), "no usable target")
	})

	t.Run("successful selection with priority", func(t *testing.T) {
		// 创建测试渠道
		chID := 1
		channelSvc.SetEnabledChannelsForTest([]*biz.Channel{
			newSelectorTestBizChannel(chID, channel.TypeOpenai, "Test Channel", "https://api.openai.com/v1", []string{"gpt-4", "gpt-3.5-turbo"}, []objects.ChannelEndpoint{createSelectorEndpoint("openai/chat/completions", "/v1")}),
		})

		runtimeAdapter := &objects.RuntimeAdapter{
			ID:               1,
			Name:             "test-adapter",
			InboundAPIFormat: "openai/chat/completions",
			Bindings: map[string]*objects.RuntimeAdapterBinding{
				"gpt-4": {
					SourceModelID: "gpt-4",
					ModelGroup: &objects.RuntimeModelGroup{
						Name: "test-group",
						Protocols: map[string]*objects.RuntimeModelGroupProtocol{
							"openai/chat/completions": {
								InboundAPIFormat: "openai/chat/completions",
								Targets: []*objects.RuntimeModelGroupTarget{
									{
										ChannelID:         chID,
										TargetModelID:     "gpt-4",
										OutboundAPIFormat: "openai/chat/completions",
										Priority:          1,
									},
								},
							},
						},
					},
					Enabled: true,
				},
			},
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "openai/chat/completions",
		}
		candidates, err := selector.Select(ctxWithAdapter, req)
		require.NoError(t, err)
		require.NotNil(t, candidates)
		require.Len(t, candidates, 1)
		assert.Equal(t, 1, candidates[0].Priority)
		assert.Equal(t, "openai/chat/completions", candidates[0].APIFormat)
	})

	t.Run("channel not enabled", func(t *testing.T) {
		// 清空渠道
		channelSvc.SetEnabledChannelsForTest([]*biz.Channel{})

		runtimeAdapter := &objects.RuntimeAdapter{
			ID:               1,
			Name:             "test-adapter",
			InboundAPIFormat: "openai/chat/completions",
			Bindings: map[string]*objects.RuntimeAdapterBinding{
				"gpt-4": {
					SourceModelID: "gpt-4",
					ModelGroup: &objects.RuntimeModelGroup{
						Name: "test-group",
						Protocols: map[string]*objects.RuntimeModelGroupProtocol{
							"openai/chat/completions": {
								InboundAPIFormat: "openai/chat/completions",
								Targets: []*objects.RuntimeModelGroupTarget{
									{
										ChannelID:         999, // 不存在的渠道
										TargetModelID:     "gpt-4",
										OutboundAPIFormat: "openai/chat/completions",
										Priority:          1,
									},
								},
							},
						},
					},
					Enabled: true,
				},
			},
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		req := &llm.Request{
			Model:     "gpt-4",
			APIFormat: "openai/chat/completions",
		}
		candidates, err := selector.Select(ctxWithAdapter, req)
		assert.Error(t, err)
		assert.Nil(t, candidates)
		assert.Contains(t, err.Error(), "no usable target")
	})
}

// TestAdapterCandidateSelector_ResolveAdapter 测试 resolveAdapter 方法。
func TestAdapterCandidateSelector_ResolveAdapter(t *testing.T) {
	selector, _, client := setupAdapterSelectorTest(t)
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)

	t.Run("from runtime adapter context", func(t *testing.T) {
		runtimeAdapter := &objects.RuntimeAdapter{
			Name: "context-adapter",
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		result, err := selector.resolveAdapter(ctxWithAdapter)
		require.NoError(t, err)
		assert.Equal(t, "context-adapter", result.Name)
	})

	t.Run("from adapter name context", func(t *testing.T) {
		// 需要先设置适配器服务中的快照
		// 这需要通过 Refresh 方法触发
		// 此测试只验证上下文查找逻辑
		ctxWithName := contexts.WithAdapterName(ctx, "test-adapter")

		result, err := selector.resolveAdapter(ctxWithName)
		// 没有刷新快照，应该失败
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("runtime adapter with empty name", func(t *testing.T) {
		runtimeAdapter := &objects.RuntimeAdapter{
			Name: "",
		}
		ctxWithAdapter := contexts.WithRuntimeAdapter(ctx, runtimeAdapter)

		result, err := selector.resolveAdapter(ctxWithAdapter)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "empty name")
	})

	t.Run("no adapter context", func(t *testing.T) {
		result, err := selector.resolveAdapter(ctx)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "runtime adapter context is missing")
	})
}

// TestHasOutboundEndpoint 测试 hasOutboundEndpoint 函数。
func TestHasOutboundEndpoint(t *testing.T) {
	testChannel := &biz.Channel{
		Channel: &ent.Channel{
			Endpoints: []objects.ChannelEndpoint{
				{APIFormat: "openai/chat/completions", Path: "/v1"},
				{APIFormat: "openai/embeddings", Path: "/v1"},
			},
		},
	}

	tests := []struct {
		name     string
		format   string
		expected bool
	}{
		{
			name:     "has endpoint",
			format:   "openai/chat/completions",
			expected: true,
		},
		{
			name:     "has endpoint - embeddings",
			format:   "openai/embeddings",
			expected: true,
		},
		{
			name:     "no endpoint",
			format:   "anthropic/messages",
			expected: false,
		},
		{
			name:     "empty format",
			format:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasOutboundEndpoint(testChannel, tt.format)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTargetSupportsRequest 测试目标能力过滤。
func TestTargetSupportsRequest(t *testing.T) {
	tests := []struct {
		name     string
		target   *objects.RuntimeModelGroupTarget
		request  *llm.Request
		expected bool
	}{
		{
			name: "stream request with stream support",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					SupportsStream: true,
				},
			},
			request: &llm.Request{
				Stream: boolPtr(true),
			},
			expected: true,
		},
		{
			name: "stream request without stream support",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					SupportsStream: false,
				},
			},
			request: &llm.Request{
				Stream: boolPtr(true),
			},
			expected: false,
		},
		{
			name: "tools request with tools support",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					SupportsTools: true,
				},
			},
			request: &llm.Request{
				Tools: []llm.Tool{{Type: "function", Function: llm.Function{Name: "test-tool"}}},
			},
			expected: true,
		},
		{
			name: "tools request without tools support",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					SupportsTools: false,
				},
			},
			request: &llm.Request{
				Tools: []llm.Tool{{Type: "function", Function: llm.Function{Name: "test-tool"}}},
			},
			expected: false,
		},
		{
			name: "image input with image modality",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					InputModalities: []string{"text", "image"},
				},
			},
			request: &llm.Request{
				Messages: []llm.Message{
					{Content: llm.MessageContent{
						MultipleContent: []llm.MessageContentPart{
							{Type: "image_url", ImageURL: &llm.ImageURL{URL: "data:image/png;base64,..."}},
						},
					}},
				},
			},
			expected: true,
		},
		{
			name: "image input without image modality",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					InputModalities: []string{"text"},
				},
			},
			request: &llm.Request{
				Messages: []llm.Message{
					{Content: llm.MessageContent{
						MultipleContent: []llm.MessageContentPart{
							{Type: "image_url", ImageURL: &llm.ImageURL{URL: "data:image/png;base64,..."}},
						},
					}},
				},
			},
			expected: false,
		},
		{
			name: "image generation with image output",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					OutputModalities: []string{"text", "image"},
				},
			},
			request: &llm.Request{
				RequestType: llm.RequestTypeImage,
			},
			expected: true,
		},
		{
			name: "image generation without image output",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					OutputModalities: []string{"text"},
				},
			},
			request: &llm.Request{
				RequestType: llm.RequestTypeImage,
			},
			expected: false,
		},
		{
			name: "default text request",
			target: &objects.RuntimeModelGroupTarget{
				Capabilities: objects.AdapterTargetCapabilities{
					SupportsStream:   true,
					SupportsTools:    true,
					InputModalities:  []string{"text"},
					OutputModalities: []string{"text"},
				},
			},
			request: &llm.Request{
				RequestType: llm.RequestTypeChat,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := targetSupportsRequest(tt.target, tt.request)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasModality 测试 hasModality 函数。
func TestHasModality(t *testing.T) {
	tests := []struct {
		name           string
		modalities     []string
		expected       string
		expectedResult bool
	}{
		{
			name:           "has text",
			modalities:     []string{"text", "image"},
			expected:       "text",
			expectedResult: true,
		},
		{
			name:           "case insensitive",
			modalities:     []string{"TEXT", "IMAGE"},
			expected:       "text",
			expectedResult: true,
		},
		{
			name:           "not found",
			modalities:     []string{"text", "image"},
			expected:       "video",
			expectedResult: false,
		},
		{
			name:           "empty modalities",
			modalities:     []string{},
			expected:       "text",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasModality(tt.modalities, tt.expected)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// boolPtr 返回布尔值的指针。
func boolPtr(b bool) *bool {
	return &b
}
