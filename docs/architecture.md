# AxonHub Adapter Gateway 架构

用途：给后续 AI 与人类开发者提供无需盲搜代码即可理解网关结构、请求链路和边界的架构总览。

## 1. 核心数据流

消费请求按以下顺序处理：

```text
消费请求
  → Adapter middleware（按 URL path 识别 adapter）
  → AdapterCandidateSelector
  → source model alias → AdapterModelBinding → Model
  → Model.Settings.ProtocolPools[inbound protocol family]
  → Channel endpoints
  → provider
```

- URL 形如 `/:adapter/v1/chat/completions`、`/:adapter/v1/responses` 或 `/:adapter/v1/messages`；动态路由把 `adapter` 写入请求上下文。
- Selector 校验 Adapter 的入站协议和 source model alias，取绑定的逻辑 Model，再从对应协议池生成可用 Channel/物理模型候选。
- 候选只保留启用的 binding、Model、Channel 和匹配入站/出站格式的 endpoint；没有可用目标时显式报错，不回退到全渠道搜索。

## 2. Model-centric 模型

**Model 是唯一的逻辑模型中心。** `Model.Settings.ProtocolPools` 的契约如下：

```text
map[string][]*ModelAssociation
key   = 协议族（openai 或 anthropic）
value = 该协议下的 channel_model、regex 等关联
```

- 一个 Model 可以同时拥有 `openai` 与 `anthropic` 协议池；协议池中的关联描述 Channel、物理模型、优先级和启用状态。
- **Adapter 只绑定 Model**：`AdapterModelBinding(source_model_id, model_id)`；Adapter 不绑定 Channel，也不保存物理模型。
- 物理目标切换只修改 Model 的协议池，刷新快照后所有引用该 Model 的 Adapter 都能使用新配置。

## 3. 协议隔离

- `openai/*` 入站请求只查 `openai` pool；`anthropic/*` 入站请求只查 `anthropic` pool。
- 完整 API format 在 `internal/server/orchestrator/adapter_selector.go` 的 `protocolPoolKey()`（当前实现名为 `normalizeProtocolPoolKey`）中归一化为协议族 key。
- **不做跨协议转换**：协议池决定可用目标及出站 endpoint；endpoint 不支持该协议时不会被加入候选。

## 4. 管理面与消费面

管理面：

```text
/admin/gateway/adapters   Adapter CRUD（owner JWT）
/admin/graphql             Model settings.protocolPools 编辑
/admin/gateway/refresh     构建并发布 Adapter 快照
```

消费面：

```text
/:adapter/v1/chat/completions
/:adapter/v1/responses
/:adapter/v1/messages
/:adapter/v1/models
```

消费入口的 MVP 默认不校验消费侧 API Key；生产环境必须通过本机绑定、反向代理、IP 白名单或上游鉴权代理隔离端口。

## 5. 快照机制

`AdapterService.Refresh` 从数据库和启用的 Channel 配置构建完整、不可变的 `AdapterSnapshot`，通过一次原子替换发布；失败时保留旧快照并返回诊断信息。请求路径只调用 `Snapshot/Resolve` 读快照，不直接读取数据库，也不会看到半成品 map 或切片。

## 6. 关键文件索引

| 文件 | 职责 |
|---|---|
| `internal/server/orchestrator/adapter_selector.go` | 校验入站协议，按 source alias、Model 与协议池选择 Channel 候选。 |
| `internal/server/biz/adapter.go` | Adapter CRUD 的业务服务、刷新流程、不可变运行时快照与诊断。 |
| `internal/objects/model.go` | Model settings、ModelAssociation 及协议池关联对象。 |
| `internal/objects/adapter.go` | RuntimeAdapter、绑定和 AdapterSnapshot 的运行时结构。 |
| `internal/server/api/adapter.go` | 动态 Adapter 的 chat/responses/messages/models 消费处理器。 |
| `internal/server/routes.go` | 注册 `/:adapter/v1` 动态路由和 Adapter middleware。 |
| `internal/server/gql/` | Model settings 与 `protocolPools` 的 GraphQL 查询、输入和 resolver。 |
| `internal/server/middleware/` | 从 URL、请求上下文和认证边界识别/传递 Adapter。 |

修改 Adapter、Model 或协议池时，先确认上述契约，再分别验证管理面保存、快照刷新和消费面路由。