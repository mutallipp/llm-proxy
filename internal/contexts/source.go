package contexts

import (
	"context"

	"github.com/mutallipp/llm-proxy/internal/ent/request"
)

// TestOriginKind 标识统一测试入口的发起配置面，用于把 source=test 请求
// 关联回对应的 Channel/Model/Adapter 历史。值与 Ent schema 中的
// test_origin_type enum 完全对应；保持小写、不可变，允许为空。
type TestOriginKind string

const (
	// TestOriginChannel 表示请求由 Channel 测试入口发起。
	TestOriginChannel TestOriginKind = "channel"
	// TestOriginModel 表示请求由逻辑 Model 测试入口发起。
	TestOriginModel TestOriginKind = "model"
	// TestOriginAdapter 表示请求由 Adapter 测试入口发起。
	TestOriginAdapter TestOriginKind = "adapter"
)

// TestOrigin 携带发起测试的实体信息，由 RequestService 在
// CreateRequest 写入 ent.Request 的 test_origin_* 列。
// ID 与 Label 都参与持久化：Label 用于历史回显（即便实体被删除也能识别）。
type TestOrigin struct {
	Kind  TestOriginKind
	ID    int
	Label string
}

// Valid 判断归属是否能安全写入 Request。无效归属不会污染普通或测试历史。
func (origin *TestOrigin) Valid() bool {
	if origin == nil || origin.ID <= 0 {
		return false
	}

	switch origin.Kind {
	case TestOriginChannel, TestOriginModel, TestOriginAdapter:
		return true
	default:
		return false
	}
}

// TestRequestCapture 在 source=test 路径上由 RequestService.CreateRequest
// 在写入数据库成功后赋值，供测试执行器把主请求 Relay ID 返回给调用方。
// 当调用方未要求捕获或 CreateRequest 未被触发时，字段保持为 nil。
type TestRequestCapture struct {
	RequestID int
	RelayID   string
}

// WithSource stores the request source in the context.
func WithSource(ctx context.Context, source request.Source) context.Context {
	container := getContainer(ctx)
	container.Source = &source

	return withContainer(ctx, container)
}

// GetSource retrieves the request source from the context.
func GetSource(ctx context.Context) (request.Source, bool) {
	container := getContainer(ctx)
	if container.Source != nil {
		return *container.Source, true
	}

	return request.SourceAPI, false
}

// GetSourceOrDefault retrieves the request source from the context, or returns the default value if it doesn't exist.
func GetSourceOrDefault(ctx context.Context, defaultSource request.Source) request.Source {
	if source, ok := GetSource(ctx); ok {
		return source
	}

	return defaultSource
}

// WithTestOrigin 记录本次请求由哪个测试配置面发起。仅在 source=test
// 时使用，其它来源不应写入。
func WithTestOrigin(ctx context.Context, origin *TestOrigin) context.Context {
	if origin == nil {
		return ctx
	}

	container := getContainer(ctx)
	container.mu.Lock()
	container.TestOrigin = &TestOrigin{Kind: origin.Kind, ID: origin.ID, Label: origin.Label}
	container.mu.Unlock()

	return withContainer(ctx, container)
}

// GetTestOrigin 返回当前请求上下文的测试发起归属；不存在时返回 nil。
func GetTestOrigin(ctx context.Context) *TestOrigin {
	container := getContainer(ctx)
	container.mu.RLock()
	defer container.mu.RUnlock()
	if container.TestOrigin == nil {
		return nil
	}

	origin := *container.TestOrigin

	return &origin
}

// SetTestRequestCapture 在 RequestService.CreateRequest 成功后写入；
// 仅用于 source=test 路径，测试入口读出后能拿到主请求定位符。
func SetTestRequestCapture(ctx context.Context, capture *TestRequestCapture) context.Context {
	if capture == nil {
		return ctx
	}

	container := getContainer(ctx)
	container.mu.Lock()
	container.TestRequestCapture = &TestRequestCapture{RequestID: capture.RequestID, RelayID: capture.RelayID}
	container.mu.Unlock()

	return withContainer(ctx, container)
}

// ClearTestRequestCapture 清除当前测试调用的主 Request 捕获，避免复用 context
// 时把上一次测试的 Relay ID 错误返回给前置拒绝结果。
func ClearTestRequestCapture(ctx context.Context) context.Context {
	container := getContainer(ctx)
	container.mu.Lock()
	container.TestRequestCapture = nil
	container.mu.Unlock()

	return withContainer(ctx, container)
}

// GetTestRequestCapture 返回最近一次 CreateRequest 写入的捕获值。
func GetTestRequestCapture(ctx context.Context) *TestRequestCapture {
	container := getContainer(ctx)
	container.mu.RLock()
	defer container.mu.RUnlock()
	if container.TestRequestCapture == nil {
		return nil
	}

	capture := *container.TestRequestCapture

	return &capture
}
