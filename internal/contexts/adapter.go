package contexts

import (
	"context"

	"github.com/mutallipp/llm-proxy/internal/objects"
)

// WithRuntimeAdapter 将当前请求使用的适配器运行时配置写入上下文。
func WithRuntimeAdapter(ctx context.Context, adapter *objects.RuntimeAdapter) context.Context {
	container := getContainer(ctx)
	container.RuntimeAdapter = adapter

	return withContainer(ctx, container)
}

// GetRuntimeAdapter 读取当前请求的适配器运行时配置。
func GetRuntimeAdapter(ctx context.Context) (*objects.RuntimeAdapter, bool) {
	container := getContainer(ctx)
	return container.RuntimeAdapter, container.RuntimeAdapter != nil
}

// WithAdapter 是 WithRuntimeAdapter 的兼容别名，便于路由中间件使用较短的名称。
func WithAdapter(ctx context.Context, adapter *objects.RuntimeAdapter) context.Context {
	return WithRuntimeAdapter(ctx, adapter)
}

// GetAdapter 是 GetRuntimeAdapter 的兼容别名。
func GetAdapter(ctx context.Context) (*objects.RuntimeAdapter, bool) {
	return GetRuntimeAdapter(ctx)
}

// WithAdapterName 仅保存适配器名称。选择器会从当前快照解析最新运行时配置。
func WithAdapterName(ctx context.Context, name string) context.Context {
	container := getContainer(ctx)
	container.AdapterName = &name

	return withContainer(ctx, container)
}

// GetAdapterName 读取上下文中的适配器名称。
func GetAdapterName(ctx context.Context) (string, bool) {
	container := getContainer(ctx)
	if container.AdapterName == nil {
		return "", false
	}

	return *container.AdapterName, true
}
