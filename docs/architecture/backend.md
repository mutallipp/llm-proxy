# AxonHub 后端技术架构与开发规范

本文描述当前源码中的后端边界、请求链路和 Adapter Gateway 约束。路径均相对于仓库根目录。

## 1. 技术栈与模块目录

| 层次 | 选型与职责 | 主要路径 |
| --- | --- | --- |
| 运行时 | Go 1.26；Uber FX 负责依赖注入和模块装配 | [`go.mod`](../../go.mod)、[`cmd/axonhub/main.go`](../../cmd/axonhub/main.go)、[`internal/server/biz/fx_module.go`](../../internal/server/biz/fx_module.go) |
| HTTP/API | Gin 路由、中间件、REST/协议兼容 handler | [`internal/server/routes.go`](../../internal/server/routes.go)、[`internal/server/api/`](../../internal/server/api/)、[`internal/server/middleware/`](../../internal/server/middleware/) |
| 数据访问 | Ent schema 驱动 ORM；SQLite 为配置示例和默认开发数据库，PostgreSQL 用于部署 | [`internal/ent/schema/`](../../internal/ent/schema/)、[`config.example.yml`](../../config.example.yml)、[`docker-compose.yml`](../../docker-compose.yml) |
| GraphQL | gqlgen 管理管理端 GraphQL schema/resolver；OpenAPI GraphQL 单独生成 | [`internal/server/gql/`](../../internal/server/gql/)、[`internal/server/gql/openapi/`](../../internal/server/gql/openapi/)、[`internal/server/gql/generate.go`](../../internal/server/gql/generate.go) |
| LLM 核心 | 统一 `llm.Request`/`llm.Response`、协议 transformer、pipeline、HTTP executor；是独立 Go module | [`llm/go.mod`](../../llm/go.mod)、[`llm/model.go`](../../llm/model.go)、[`llm/pipeline/`](../../llm/pipeline/)、[`llm/transformer/`](../../llm/transformer/) |
| 业务编排 | 渠道候选选择、负载均衡、重试、请求/用量持久化 | [`internal/server/orchestrator/`](../../internal/server/orchestrator/)、[`internal/server/biz/`](../../internal/server/biz/) |

数据库方言由 `AXONHUB_DB_DIALECT`/`AXONHUB_DB_DSN` 或配置文件提供：开发示例使用 SQLite，Compose/Helm 示例使用 PostgreSQL。不要在代码中写死个人数据库地址。

## 2. 请求链路

### 2.1 普通协议入口

`internal/server/routes.go` 先挂载全局 IP 访问控制、访问日志、Ent client、日志 tracing、metrics，再按 API 组挂载超时、IP blocklist、API key、`WithSource`、`WithThread`、`WithTrace`。OpenAI、Anthropic 等协议 handler 位于 [`internal/server/api/`](../../internal/server/api/)。

### 2.2 Adapter 入口与统一编排

动态路由为 `/:adapter/v1`，实际注册在 [`internal/server/routes.go`](../../internal/server/routes.go)，消费请求的中间件顺序为：

1. `WithTimeout`、`WithIPBlocklist`；
2. `WithAdapterConsumerInterceptor`：读取消费端凭证到 context，并移除 `Authorization`/`x-api-key`/`api-key`，避免凭证传播给渠道；
3. `WithAdapterRoute`：从不可变 Adapter snapshot 解析 `:adapter`，写入 `RuntimeAdapter` context；不存在或禁用统一返回 404；
4. `WithAdapterNoAuthPersistence`：必要时创建/复用 no-auth API key，使请求仍可持久化；
5. `WithSource`、`WithThread`、`WithTrace`。

协议门面在 [`internal/server/api/adapter.go`](../../internal/server/api/adapter.go)：`POST /chat/completions`、`POST /responses`、`POST /messages` 分别复用 OpenAI/Anthropic handler，只替换为 `AdapterCandidateSelector`，因此解析和流式写出逻辑不重复实现。

核心处理顺序如下（同步响应和流式响应共用 pipeline）：

```text
Gin routes/middleware
  -> api handler
  -> inbound transformer: httpclient.Request -> llm.Request
  -> inbound middleware: 配额/API key/模型映射/候选选择/提示词/持久化
  -> CandidateSelector: 解析模型组、目标池和渠道
  -> PersistentOutboundTransformer: 选择当前候选，写入实际模型和渠道选项
  -> outbound transformer: llm.Request -> httpclient.Request
  -> HTTP executor/provider（非流式或流式）
  -> provider response/stream -> outbound transformer -> llm.Response
  -> usage、性能、请求执行和 trace 持久化
  -> inbound transformer -> 客户端响应/客户端流
```

对应实现路径：

- pipeline 入口和 retry 循环：[`llm/pipeline/pipeline.go`](../../llm/pipeline/pipeline.go) 的 `Process`/`processRequest`；`Inbound.TransformRequest` 先将外部协议转成统一请求，`Outbound.TransformRequest` 再生成供应商 HTTP 请求。
- Transformer 接口契约：[`llm/transformer/interfaces.go`](../../llm/transformer/interfaces.go)。`Inbound` 负责“客户端协议 ↔ unified response”，`Outbound` 负责“unified request/response ↔ provider 协议”，入站和出站格式不可混用。
- 编排及候选：[`internal/server/orchestrator/orchestrator.go`](../../internal/server/orchestrator/orchestrator.go) 的 `ChatCompletionOrchestrator.Process`；[`internal/server/orchestrator/candidates.go`](../../internal/server/orchestrator/candidates.go) 定义 `CandidateSelector` 和 `ChannelModelsCandidate`。
- 持久化包装：[`internal/server/orchestrator/transformer.go`](../../internal/server/orchestrator/transformer.go)、[`internal/server/orchestrator/inbound.go`](../../internal/server/orchestrator/inbound.go)、[`internal/server/orchestrator/outbound.go`](../../internal/server/orchestrator/outbound.go)。它们记录 Request/RequestExecution、响应、流块、用量和性能，并在切换候选时重置执行状态。
- 负载均衡：`NewLoadBalancer` 在 [`internal/server/orchestrator/orchestrator.go`](../../internal/server/orchestrator/orchestrator.go) 装配 adaptive、failover、circuit-breaker、round-robin 策略；策略实现位于 [`internal/server/orchestrator/`](../../internal/server/orchestrator/)。
- 重试：[`llm/pipeline/pipeline.go`](../../llm/pipeline/pipeline.go) 先尝试同渠道 `ChannelRetryable`，再通过 `Retryable` 切换渠道；可配置最大次数、延迟、空响应检测和超时。具体错误判定在 [`internal/server/orchestrator/retry.go`](../../internal/server/orchestrator/retry.go)。
- Provider 执行：[`llm/pipeline/executor.go`](../../llm/pipeline/executor.go) 的 `Executor.Do`/`DoStream`，生产实现由 `llm/httpclient` 提供。
- 用量/追踪：请求链路由 [`internal/server/middleware/logging.go`](../../internal/server/middleware/logging.go)、[`internal/server/middleware/trace.go`](../../internal/server/middleware/trace.go) 建立 request/trace context；用量写入由 [`internal/server/orchestrator/request.go`](../../internal/server/orchestrator/request.go) 和 [`internal/server/orchestrator/outbound.go`](../../internal/server/orchestrator/outbound.go) 调用 `UsageLogService`，trace 聚合在 [`internal/server/biz/trace.go`](../../internal/server/biz/trace.go)。

## 3. Model-centric Adapter Gateway（2026-08-02）

### 3.1 数据流与配置关系

Model 是 Adapter Gateway 的唯一逻辑模型中心，配置和运行时数据流为：

```text
消费请求
  -> Adapter middleware（识别 adapter 与固定入站协议）
  -> source_model_id
  -> AdapterModelBinding.model_id
  -> Model.Settings.ProtocolPools[protocolPoolKey]
  -> ModelAssociation(channel_model、物理模型、priority、enabled)
  -> 启用 Channel 的完整 apiFormat endpoint
  -> provider
```

- `Adapter` 对外提供 `/{adapter}/v1`，只维护消费方看到的 `source_model_id -> model_id` binding；不绑定 Channel、物理模型或出站格式。
- 一个 `Model` 可以同时维护 `openai`、`anthropic` 等独立协议池；协议池关联负责声明 Channel、物理模型、优先级和启用状态。
- 目标切换只修改 Model 的协议池并刷新快照，所有引用该 Model 的 Adapter 都读取新目标。
### 3.2 协议隔离与格式边界

- 入站请求使用完整格式，例如 `openai/chat_completions`、`openai/responses`、`anthropic/messages`；`normalizeProtocolPoolKey`（概念名 `protocolPoolKey`）将其归一化为协议族 key `openai` 或 `anthropic`。
- 协议池 key 与 Channel 的 `endpoints[].apiFormat` 是不同层级：协议池保存协议族，endpoint 保存完整格式，例如 `openai/chat_completions`。不能用完整 endpoint 字符串作为 pool key，也不能把 pool key 当作 endpoint 直接比较。
- 前端通过 `channelSupportsProtocolPool` 按 `openai/`、`anthropic/` 前缀过滤可选 Channel；运行时仍需确认启用 Channel 存在匹配请求格式的完整 endpoint。
- Adapter 只查与自身入站协议对应的 pool，不跨协议兜底、不隐式转换，也不回退到全渠道选择；无 pool、无 binding 或无可用 endpoint 时应返回明确诊断。

### 3.3 渠道协议能力声明与自动派生（2026-08-05）

- 协议×模型能力归渠道层声明：Channel 的 `protocol_capabilities`（`DeclaredProtocols` + 逐模型 `Protocols`），唯一写入面 `saveChannelCapabilities`（含端点能力校验与协议族白名单校验；`Create/UpdateChannelInput` 不含该字段）。
- 保存后 `protocol_pool_derivation.go` 对账，为同名逻辑模型物化 `settings.protocolPools` 的 auto 关联（`auto: true`、默认禁用）；对账只碰 auto 条目，手动条目（含存量显式关联与 developer 继承）永不被派生触碰。
- auto 条目一经启用即转手动；启用（首次/批量/重启用）统一走 `model_capability_validation.go` 校验，钩子覆盖 CreateModel/UpdateModel/BulkCreateModels 三条 settings 写入路径；服务端拒绝删除现存 auto 条目，入站 auto/disabledReason 以服务端现存状态为准。
- 派生触发点：saveChannelCapabilities、SaveChannelEndpoints 端点复核（失去端点能力支撑的声明按撤销处理）、auto_sync 模型增删、Delete/BulkDeleteChannels 清理、deriveModelAssociations（按模型名增量、只增不撤）。DuplicateChannel 不复制能力声明。
- 运行时路由链路（orchestrator）不变：仍读 `EffectiveModelProtocolPools`，auto 与手动条目运行期语义一致。

### 3.4 Snapshot、Refresh 与状态规则

[`internal/server/biz/adapter.go`](../../internal/server/biz/adapter.go) 的 `AdapterService` 使用 `atomic.Value` 保存 [`objects.AdapterSnapshot`](../../internal/objects/adapter.go)：

- `Refresh` 从数据库读取启用的 Adapter、binding、Model 和 `Model.Settings`，结合 `ChannelService.GetEnabledChannels()` 构建完整 snapshot；全部校验通过后一次性原子替换。
- `Snapshot`、`Resolve`、`ListModels` 只读当前 snapshot，不在请求路径查数据库。发布后的 map、slice 和对象视为只读，调用方不得修改。
- 刷新失败保留旧 snapshot，记录 `LastRefreshError`；成功版本递增并记录 `RefreshedAt`/`LastSuccessfulRefreshAt`，诊断信息随结果返回。
- 配置保存不是运行时生效的充分条件：写库成功后必须调用 `AdapterService.Refresh` 或管理端 `POST /admin/gateway/refresh`，并检查 `snapshot_version`/`diagnostics`。
- 禁用的 Adapter、Model 或协议池关联不会进入运行时 snapshot；重新启用后必须再次 Refresh。请求解析失败不得回退到普通全渠道选择。

### 3.5 关键文件索引

| 文件 | 职责 |
|---|---|
| [`internal/server/orchestrator/adapter_selector.go`](../../internal/server/orchestrator/adapter_selector.go) | 校验 Adapter 入站协议，归一化 pool key，并从绑定 Model 的协议池生成 Channel 候选。 |
| [`internal/server/biz/adapter.go`](../../internal/server/biz/adapter.go) | Adapter CRUD、Model binding、刷新流程、不可变 runtime snapshot 和诊断。 |
| [`internal/objects/model.go`](../../internal/objects/model.go) | Model settings、ModelAssociation 和协议池数据结构。 |
| [`internal/objects/adapter.go`](../../internal/objects/adapter.go) | RuntimeAdapter、binding 和 AdapterSnapshot 结构。 |
| [`internal/server/api/adapter.go`](../../internal/server/api/adapter.go) | 动态 Adapter 的 chat/responses/messages/models 消费处理器。 |
| [`internal/server/routes.go`](../../internal/server/routes.go) | 注册 `/:adapter/v1` 动态路由和 Adapter middleware。 |
| [`internal/server/gql/`](../../internal/server/gql/) | Model settings 与 `protocolPools` 的 GraphQL 查询、输入和 resolver。 |
| [`internal/server/biz/channel_capability.go`](../../internal/server/biz/channel_capability.go) | 渠道协议能力声明读写、端点能力校验、端点复核与派生触发。 |
| [`internal/server/biz/protocol_pool_derivation.go`](../../internal/server/biz/protocol_pool_derivation.go) | 协议池 auto 派生对账引擎（纯函数 reconcile + 单事务 merge-write）。 |
| [`internal/server/biz/model_capability_validation.go`](../../internal/server/biz/model_capability_validation.go) | auto 条目启用的统一能力校验。 |

### 3.6 维护入口

- Adapter/Model binding、GID 和协议池格式规则：[`../../.agent/rules/adapter-model-binding.md`](../../.agent/rules/adapter-model-binding.md)。
- Ent/GraphQL schema 与生成代码：[`../../.agent/rules/ent-graphql.md`](../../.agent/rules/ent-graphql.md)。
- 定向测试和浏览器/curl 验收：[`../../.agent/rules/e2e.md`](../../.agent/rules/e2e.md)。
- 前端页面状态与 GraphQL 查询：[`frontend.md`](frontend.md)。
