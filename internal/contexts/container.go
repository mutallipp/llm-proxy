package contexts

import (
	"context"
	"sync"

	"github.com/mutallipp/llm-proxy/internal/ent"
	"github.com/mutallipp/llm-proxy/internal/ent/request"
	"github.com/mutallipp/llm-proxy/internal/objects"
)

// contextContainer 保存请求上下文中的共享值。
type contextContainer struct {
	ProjectID     *int
	TraceID       *string
	RequestID     *string
	OperationName *string
	APIKey        *ent.APIKey
	User          *ent.User
	Source        *request.Source
	Thread        *ent.Thread
	Trace         *ent.Trace
	Errors        []error
	mu            sync.RWMutex

	// ChannelAPIKey 保存发往渠道请求的 API Key，不是消费端 API Key。
	ChannelAPIKey *string

	// AdapterConsumerAPIKey 保存适配器消费端携带的 API Key，仅存在于请求上下文中。
	// 一期不校验、不落库，也不会作为渠道凭证转发。
	AdapterConsumerAPIKey *string

	// RuntimeAdapter 保存请求选中的不可变适配器配置。
	RuntimeAdapter *objects.RuntimeAdapter
	AdapterName    *string
}

// getContainer 读取上下文容器；不存在时创建一个新容器。
func getContainer(ctx context.Context) *contextContainer {
	if container, ok := ctx.Value(containerContextKey).(*contextContainer); ok {
		return container
	}

	// 首次使用时创建容器；调用方随后会负责将其写回上下文。
	container := &contextContainer{}

	return container
}

// withContainer 在上下文尚未保存容器时写入容器。
func withContainer(ctx context.Context, container *contextContainer) context.Context {
	if ctx.Value(containerContextKey) == nil {
		return context.WithValue(ctx, containerContextKey, container)
	}

	return ctx
}
