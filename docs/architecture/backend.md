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

## 3. Adapter Gateway 模型与运行时

### 3.1 配置关系

实体 schema 位于 [`internal/ent/schema/`](../../internal/ent/schema/)：

```text
Adapter (消费入口名、inbound_api_format、状态)
  1 -> N AdapterModelBinding (source_model_id -> model_group_id)
  1 -> 1 ModelGroup (逻辑模型组、选择策略、状态)
  1 -> N ModelGroupProtocol (inbound_api_format)
  1 -> N ModelGroupTarget (channel_id、target_model_id、outbound_api_format、priority、capabilities)
```

- `Adapter` 是对外 `/{adapter}/v1` 的入口；`inbound_api_format` 决定该入口接受的协议。
- `AdapterModelBinding` 把消费方看到的 `source_model_id` 绑定到一个 `ModelGroup`。
- `ModelGroupProtocol` 按入站协议隔离目标池；同一模型组可为不同入站协议配置不同目标。
- `ModelGroupTarget` 将目标模型映射到具体 `Channel`，并明确出站协议 `outbound_api_format`。入站协议和出站协议必须独立配置，不能用入站格式推断渠道格式。

### 3.2 Snapshot、Refresh 与状态规则

[`internal/server/biz/adapter.go`](../../internal/server/biz/adapter.go) 的 `AdapterService` 使用 `atomic.Value` 保存 [`objects.AdapterSnapshot`](../../internal/objects/adapter.go)：

- `Refresh` 从数据库读取启用的 Adapter、Binding、ModelGroup、Protocol、Target，并结合 `ChannelService.GetEnabledChannels()` 构建完整对象；所有校验成功后一次性原子替换。
- `Snapshot`、`Resolve`、`ListModels` 只读当前 snapshot，不在请求路径查数据库。发布后的 map、slice、对象视为只读；不要修改返回值。
- 刷新失败保留旧 snapshot，记录 `LastRefreshError`；成功版本递增并记录 `RefreshedAt`/`LastSuccessfulRefreshAt`，诊断信息随结果返回。
- 配置保存不是运行时生效的充分条件：`UpdateAdapter`、`UpdateModelGroup` 等写库成功后必须调用 `AdapterService.Refresh`；也可调用管理端 `POST /admin/gateway/refresh`。刷新失败要返回错误，不能假装新配置已生效。
- 父对象禁用时允许保留子配置：例如禁用 `ModelGroup` 不删除其 Protocol/Target；但 `loadSnapshot` 会将父对象禁用下的子配置排除，不能进入运行时 snapshot。重新启用父对象并 Refresh 后才恢复。
- Adapter 请求解析失败不得回退到普通全渠道选择；[`internal/server/orchestrator/adapter_selector.go`](../../internal/server/orchestrator/adapter_selector.go) 要求存在 runtime adapter、匹配入站格式、有效 binding/protocol/target 和启用渠道出站端点。

## 4. ModelGroupTarget 能力与兼容规则

`capabilities` 定义在 [`internal/objects/adapter.go`](../../internal/objects/adapter.go)，管理 API DTO 和枚举校验在 [`internal/server/api/gateway.go`](../../internal/server/api/gateway.go)。

- `supports_tools`：目标是否接收 tools。请求包含 tools 时，`false` 的目标被过滤。
- `supports_stream`：旧数据兼容字段，声明目标是否支持流式。仅当 `stream_policy` 为空时，它参与流式请求过滤。
- `stream_policy`：目标级策略，枚举为：
  - `unlimited`：跟随下游请求；不因 `supports_stream=false` 过滤；
  - `require`：目标必须以流式向下游请求。客户端请求非流式时，在支持自动聚合的协议上由 [`internal/server/orchestrator/outbound.go`](../../internal/server/orchestrator/outbound.go) 强制 provider 流式并聚合后返回；不支持自动聚合的请求不可选该目标；
  - `forbid`：禁止流式；客户端明确要求流式时过滤。
- `stream_policy` 为空表示历史数据：继续按 `supports_stream` 判断，避免升级后改变旧配置语义。
- 目标策略优先于渠道级策略；没有目标级策略时才回退到渠道策略。统一筛选逻辑在 [`internal/server/orchestrator/adapter_selector.go`](../../internal/server/orchestrator/adapter_selector.go) 和 [`internal/server/orchestrator/candidates_stream_policy.go`](../../internal/server/orchestrator/candidates_stream_policy.go)。
- 能力筛选还会检查输入/输出 modality（image/video/audio）；不要只修改布尔字段而忽略请求类型和内容模态。

## 5. 已验证的关键坑与操作约束

1. **GraphQL ID 不是 REST numeric ID。** GraphQL 返回的 Channel global ID（如 `gid://axonhub/Channel/123`）不能直接作为 `channel_id`。前端必须用 [`frontend/src/lib/utils.ts`](../../frontend/src/lib/utils.ts) 的 `extractNumberID`/`extractNumberIDAsNumber` 提取数字；模型组页面已有示例：[`frontend/src/features/model-groups/index.tsx`](../../frontend/src/features/model-groups/index.tsx)。后端 `TargetInput.ChannelID` 是正整数。
2. **保存配置后必须刷新运行时。** 数据库更新只改变持久化配置；Adapter 请求读的是 atomic snapshot。管理 API 更新成功后要 Refresh，并检查返回的 `snapshot_version`/`diagnostics`。
3. **禁用父对象不等于删除子配置。** 保留子配置便于恢复，但禁用的 Adapter/ModelGroup/Protocol/Target 不应进入 snapshot；验证时同时看数据库配置和 `/admin/gateway/runtime` 当前版本。
4. **代理由部署环境注入。** Docker/VPN 场景通过 `AXONHUB_HTTP_PROXY`、`AXONHUB_HTTPS_PROXY`、`AXONHUB_ALL_PROXY`、`AXONHUB_NO_PROXY` 等环境变量注入（见 [`docker-compose.yml`](../../docker-compose.yml)）。不要把个人 VPN 地址、token 或本地代理写入代码、配置样例或提交。

## 6. 后端开发规范

### 6.1 Ent schema、生成代码与数据变更

- 业务实体先改 [`internal/ent/schema/`](../../internal/ent/schema/)，字段、索引、edge、软删除语义以 schema 为准；`internal/ent/*.go` 是生成代码，禁止手改。
- 变更 schema 后按仓库生成入口执行 `make generate`（其内部进入 `internal/server/gql` 执行 `go generate`），必要时同时生成 OpenAPI；生成文件纳入同一变更审查。本文任务以外不要用手工复制生成结果替代生成流程。
- 需要原子更新多个实体时使用 [`internal/server/biz/abstract.go`](../../internal/server/biz/abstract.go) 的 `RunInTransaction`。在事务上下文中用 `entFromContext`/`ent.FromContext` 获取事务 client；函数返回错误必须回滚，提交失败也必须向上返回。
- 软删除实体要遵守 schema mixin 和现有 service 查询条件，不要用物理删除绕开业务约束；父子配置的保留/快照规则见上文。

### 6.2 API DTO、错误和状态码

- API 层负责 JSON binding、枚举/数值/必填校验和 DTO 转换；业务层接收明确的 params，不让 Gin request 或 GraphQL generated model 渗透到 service。Adapter Gateway 的 DTO 示例见 [`internal/server/api/gateway.go`](../../internal/server/api/gateway.go) 的 `UpdateModelGroupRequest`、`TargetInput` 和 `convertProtocolInputs`。
- REST 输出使用显式 DTO/JSON 结构，不直接把 Ent entity 当公共协议模型；GraphQL global ID 与 REST numeric ID 在边界处转换。
- 参数格式、枚举、资源不存在：返回 400/404；认证或权限失败遵循认证 middleware；业务/数据库/刷新失败返回 500。统一 JSON 错误形状和 Gin 中止方式参考 [`internal/server/middleware/error.go`](../../internal/server/middleware/error.go)；不要把内部堆栈或数据库细节返回给客户端。
- 更新配置后返回刷新版本、时间和 diagnostics（如适用），使调用方能确认“已写库”与“已发布运行时”不是同一件事。

### 6.3 验证清单

按变更风险选择验证，不以未执行的命令代替证据：

1. 先阅读调用方、schema、middleware、handler、service 和相关测试，确认协议/事务/快照约定。
2. schema 或生成代码变更：执行仓库规定的生成命令并检查 diff；GraphQL/OpenAPI 变更同时验证对应生成入口。
3. API/编排变更：补充或运行覆盖入站协议、候选过滤、流式策略、重试/切换、错误状态和持久化的测试；涉及真实 provider 时使用已有 integration test 配置，不提交凭证。
4. Adapter 配置变更至少验证：global ID 转数字、启用/禁用父对象、保存后 snapshot version 变化、无可用 target、tools/stream/modalities 过滤、`require` 自动聚合和 `forbid` 流式拒绝。
5. 提交前检查 `git diff --check`；不要在未明确授权时运行 build、lint、test 或重启开发服务器。
