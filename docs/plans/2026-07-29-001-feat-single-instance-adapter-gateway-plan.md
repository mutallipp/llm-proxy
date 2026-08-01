---
title: 单实例适配器网关 - Plan
type: feat
date: 2026-07-29
topic: single-instance-adapter-gateway
artifact_contract: ce-unified-plan/v1
artifact_readiness: implementation-ready
product_contract_source: ce-brainstorm
---

# 单实例适配器网关 - Plan

## Goal Capsule

- **目标**：将 AxonHub 收敛为单实例、单用户、按客户端适配器接入的 AI 模型网关。消费层只使用稳定的 Model ID/别名，Model 统一定义能力、价格和渠道目标，Adapter 只负责入口协议。
- **当前边界**：本计划覆盖 Pi、Claude Code、Codex 三类适配器，保留 AxonHub 的渠道、协议转换、健康检查、重试和故障转移能力；不覆盖企业多租户能力。
- **产品权威**：Model 是唯一模型中心；ModelCard 定义统一对外能力，ModelSettings.Associations 定义渠道物理模型和优先级；消费端不依赖具体渠道或 `targetModelId`。
- **开放阻塞**：实现阶段仍需以实际客户端协议样例确认 Pi、Claude Code、Codex 的入站字段、流式事件和错误语义；这不改变当前的产品边界。

## Product Contract

### Summary

AxonHub 将从带登录和多租户的平台收敛为单实例适配器网关。每个适配器可以使用不同的入站协议，但统一暴露 Model ID；Model 通过已有的渠道关联配置路由到多个真实物理模型，并按优先级自动故障转移。

### MVP Boundary

一期目标是基于当前 AxonHub 链路快速打通最小可用路径，而不是一次完成完整平台裁剪：

- 保留 User、Project、APIKey、Role、OIDC 等旧 Ent schema，先从 Adapter 消费请求链路解除依赖。
- 所有 Adapter 使用统一 `AdapterConsumerInterceptor`；消费层可携带自己的 API Key，但一期不校验、不落库、不转发、不强制存在。
- 不实现 global consumer API Key、Adapter 级 API Key、消费 token 和新的管理 token；管理接口复用当前管理鉴权或本机访问约束。
- 优先完成动态 Adapter 路由、固定入站协议、Model 绑定、ModelCard 能力与统一价格、已有渠道关联的优先级故障转移和热刷新。
- ModelGroup 不再作为新的配置中心；已有 ModelGroup 配置纳入迁移计划，迁移完成后下线重复 API、页面和路由链路。
- 暂不新增独立 Alias/Variant 层；不同能力的对外别名使用独立 Model 记录。
- 暂不为兼容性以外的能力重写现有 Channel 编排链路。

### Problem Frame

当前 AxonHub 的请求入口以通用协议路径和 API Key 上下文为中心，模型映射分散在 API Key Profile、渠道设置和模型关联中。目标使用者需要让 Pi、Claude Code、Codex 等客户端稳定地使用 `fast`、`opus` 等逻辑模型，同时可以在后台切换 Kiro、Claude Code、转转站等真实目标，而不修改消费端配置，也不受用户、项目和租户模型约束。

### Key Decisions

- **Adapter 是公开入口实例，不是固定客户端类型**：（session-settled: user-approved — Adapter 名称就是消费层 URL 路径）。创建 Adapter=`pi-anthropic` 后，消费入口就是 `/pi-anthropic/v1/`；`pi`、`cc`、`codex` 只是命名或配置模板，不是写死的路由。
- **每个 Adapter 只绑定一个入站协议**：（session-settled: user-approved — 同一个客户端可以创建多个 Adapter，例如 Pi 分别创建 `pi-anthropic` 和 `pi-openai`，但单个入口的协议必须固定）。
- **Model 是唯一模型中心**：（session-settled: user-directed — 选择现有 Model 而非新增 ModelGroup 作为能力、价格、别名和渠道关联的中心，因为 ModelCard 与 ModelSettings.Associations 已经表达这些业务语义，继续维护 ModelGroup 会造成重复模型层）。
- **每个对外别名就是独立 Model**：（session-settled: user-directed — 选择独立 Model 记录而非 Alias/Variant 子层，因为不同上下文、能力或渠道目标需要独立 ModelCard 和关联配置）。例如 `gpt-5.6-luna` 与 `gpt-5.6-luna-1m` 是两个 Model。
- **ModelCard 是统一对外能力契约**：（session-settled: user-directed — 同一个 Model 的 context、output limit、模态、工具/推理能力和对外价格必须一致；能力不一致的渠道不能被塞进同一个 Model）。
- **ModelSettings.Associations 负责物理目标和优先级**：（session-settled: user-approved — 复用已有 Model 关联能力配置多个 Channel、物理模型和 priority，避免 Adapter 再维护一套目标池）。
- **价格分为对外价格和渠道实际价格**：（session-settled: user-directed — Model 保存统一对外价格，渠道价格仅用于内部成本/计费；价格差异不自动拆分 Model）。
- **业务配置继续持久化在数据库**：（session-settled: user-approved — 选择数据库作为 Adapter、Model、ModelCard、ModelSettings.Associations 和 Channel 配置的持久化来源；YAML 只承担服务启动和基础设施配置）。
- **ModelGroup 进入迁移下线范围**：（session-settled: user-directed — 选择停止扩展并迁移下线 ModelGroup 配置链路，因为它与现有 Model/Associations 重复）。
- **一期消费入口默认不校验 API Key**：（session-settled: user-approved — 所有 Adapter 共用一个拦截器，读取消费层传入的 API Key 但一期不校验、不落库、不转发到上游）。
- **保留简化管理能力**：（session-settled: user-approved — 保留单实例后台配置入口；一期复用当前管理鉴权或限制管理接口只允许本机访问，不为消费入口新增认证改造）。

### Existing Context

- 现有服务路由集中在 `internal/server/routes.go`，通用 API 入口挂载认证、请求来源、线程和 trace 中间件；现有 `/v1`、`/anthropic/v1`、Gemini 等协议入口并非按客户端适配器隔离。
- `internal/ent/schema/channel.go` 已持久化渠道类型、凭证、支持模型、渠道级设置和出站端点。
- `internal/objects/channel.go` 中的 `ChannelSettings.ModelMappings` 和 `internal/server/biz/channel_llm.go` 的 `GetModelEntries` 已支持单渠道模型别名到实际模型的转换，但它不表达可跨适配器复用的模型目标池。
- `internal/ent/schema/api_key.go` 和 `internal/server/orchestrator/model_mapper.go` 已支持 API Key Profile 级模型映射；该能力依赖 API Key、项目和用户上下文，不作为新的 ModelGroup 语义继续扩展。
- `internal/ent/schema/model.go` 已有全局唯一 `model_id`、`model_card` 和 `settings`；`internal/objects/model.go` 的 `ModelCard` 已包含上下文、输出限制、模态、推理、工具和成本字段，`ModelSettings.Associations` 已包含渠道模型、渠道 ID 和 priority。
- `ChannelService` 已有启用渠道缓存和刷新机制，渠道运行时对象会预计算模型入口和出站转换器；Model 关联配置需要复用同等明确的更新生效边界。
- 当前 Adapter MVP 新增的 ModelGroup 链路与上述 Model/Associations 存在职责重叠，后续以迁移和下线为处理方式，不再新增能力字段。

### Requirements

#### Adapter 接入

- **R1**：系统必须按 Adapter 提供独立的对外 API 节点，使 Pi、Claude Code、Codex 可以使用不同的路径和入站协议。
- **R2**：每个 Adapter 必须声明唯一的入站协议和启用状态；所有 Adapter 路由统一经过消费拦截器。拦截器一期只提取消费层传入的 API Key，不校验、不落库、不转发；入站协议转换后的请求必须进入同一套模型组路由和渠道编排流程。
- **R3**：Adapter 暴露的模型列表和请求校验必须只呈现该 Adapter 允许使用的逻辑模型 ID，不得把渠道真实模型 ID 作为消费端的必需配置。

#### 逻辑模型与物理渠道

- **R4**：系统必须以现有 Model 作为唯一逻辑模型中心；`model_id` 是消费端使用的稳定模型 ID，也是别名的唯一标识。
- **R5**：Model 必须保存统一的 ModelCard 能力契约，包括上下文限制、最大输出、输入输出模态、工具/推理能力和统一对外价格；同一个 Model 不得同时代表能力不同的物理目标。
- **R6**：Model 的渠道关联必须表达 Channel、真实物理模型 ID、优先级、启用状态和请求条件；一个 Model 可以关联多个渠道并按 priority 故障转移。
- **R7**：能力不同的模型必须使用独立 Model ID，例如 `gpt-5.6-luna` 与 `gpt-5.6-luna-1m`；不新增独立 Alias/Variant 层，不通过多个目标能力交集生成新的对外契约。

#### 路由与故障转移

- **R8**：Adapter 必须直接绑定允许暴露的 Model ID；Adapter 不得要求消费端传入 Channel 或物理模型 ID。
- **R9**：Model 的渠道关联默认必须按照 priority 选择首选目标，并在目标发生可重试失败、超时、限流或熔断时切换到下一个可用目标。
- **R10**：Model 切换渠道目标后，响应对消费端必须继续使用请求中的逻辑 Model ID；消费端不得依赖实际目标模型名。
- **R11**：渠道已有的出站协议转换、请求覆盖、流式转换和错误转换能力必须继续适用于 Model 的渠道关联；Model 中心路由不得绕过 Channel 的出站适配能力。

#### 数据持久化与配置生效

- **R12**：Adapter、Model、ModelCard、ModelSettings.Associations 和 Channel 的业务配置必须持久化在数据库，并在服务重启后恢复；现有 ModelGroup 数据在迁移完成前保留为兼容数据，但不再作为新配置来源。
- **R13**：管理端修改 ModelCard、Model 渠道关联、目标优先级、Adapter 绑定或适配器协议后，运行时必须能够刷新相关缓存并让后续请求使用新配置，不要求消费端或 Agent 重启。
- **R14**：配置刷新必须保持一致性：单次变更不得产生部分 Adapter 绑定、部分 Model 关联或旧新配置混用的可观察状态。
- **R15**：配置更新失败时，系统必须保留上一次可用的运行时配置，并向管理端返回可定位的错误；不得静默切换到空 Model。

#### 单实例边界与鉴权

- **R16**：消费请求不得依赖用户登录、JWT 用户上下文、项目上下文、租户上下文、角色权限或用户级 API Key。
- **R17**：一期不实现消费 API Key 校验；统一拦截器允许消费层携带自己的 API Key，但默认不要求有效值。后续可增加全局 API Key 和 Adapter 级 API Key 校验开关，不重新引入用户级密钥管理。
- **R18**：一期管理接口继续复用当前项目管理鉴权或本机访问约束；不把消费层 API Key 当作管理凭证。
- **R19**：即使一期消费入口不校验 API Key，也必须在路由和代码层明确区分消费接口、管理接口和健康检查接口。

#### 简化管理能力

- **R20**：管理端必须能够维护 Adapter、Model 的 ModelCard、统一对外价格、渠道物理模型关联、优先级和启用状态。
- **R21**：管理端必须能够展示一个 Model 当前绑定的渠道目标顺序、目标健康状态和最近一次切换原因，帮助使用者确认模型是否已切换。
- **R22**：管理端必须提供显式配置刷新或等价的自动刷新反馈，使使用者可以确认数据库配置已经进入运行时。
- **R23**：管理端不再提供用户、租户、项目、角色、OIDC 登录、个人 API Key 和企业权限管理功能。

#### 兼容与可观测性

- **R24**：每次请求的日志和追踪信息必须同时记录 Adapter、Model ID、实际渠道和 `targetModelId`，但响应和消费端可见模型必须保持 Model ID。
- **R25**：请求失败时，错误信息必须区分入站协议错误、Model 无可用渠道目标、目标渠道失败和配置刷新失败，便于判断是客户端、路由还是上游问题。
- **R26**：Pi、Claude Code、Codex 的首批适配器必须分别具备代表性请求、流式请求、工具调用和错误场景的兼容性验收样例；具体字段以实际客户端协议为准。

### Core Flow

Adapter 是由管理端创建的公开消费入口，不是固定的 `pi`、`cc`、`codex` 三类系统对象。Adapter 的 `name/slug` 同时决定 URL 路径，例如 Adapter=`pi-anthropic` 对应 `/pi-anthropic/v1/...`。每个 Adapter 只能声明一个入站 APIFormat；同一个客户端可以创建多个 Adapter，例如 Pi 同时创建 `pi-anthropic` 和 `pi-openai`。

```mermaid
flowchart TD
    A[客户端请求] --> B[/{adapter}/v1 路由]
    B --> C[按 adapter name/slug 查 Adapter]
    C --> D[校验唯一入站 APIFormat]
    D --> E[入站协议转换]
    E --> F[Model ID / alias]
    F --> G[Adapter 绑定 Model]
    G --> H[读取 ModelCard 与 ModelSettings.Associations]
    H --> I[按 priority 与健康状态选择渠道目标]
    I --> J[Channel + physical model ID + outbound APIFormat]
    J --> K[选择 Channel 对应 endpoint]
    K --> L[出站协议转换]
    L --> M[上游渠道]
    M --> N[响应恢复为逻辑模型 ID]
```

Adapter 仍然只声明一个入站 APIFormat；Model 本身不再复制一套 ModelGroupProtocol。渠道关联继续复用现有 Channel endpoint 选择和请求格式条件。如果某个能力或协议组合不能共用同一个 Model，则创建独立 Model ID/别名，而不是在路由时动态聚合。

### Out of Scope

- 多用户、多租户、项目隔离、RBAC、OIDC 和用户登录流程。
- 用户级 API Key 生命周期、用户级配额和按租户计费策略。
- 让消费端直接选择或依赖 `targetModelId`。
- 默认对不同能力或不同协议目标进行无条件轮询。
- 通过多个目标的能力交集自动生成 ModelCard；ModelCard 由 Model 配置统一维护。
- 继续扩展 ModelGroup 作为新的模型配置中心；ModelGroup 迁移完成后的公开 API、页面和配置链路属于下线路径。
- 重写 AxonHub 已有的渠道凭证、出站协议转换、流式处理、重试、熔断和请求记录能力。
- 在没有真实客户端协议样例前，臆造 Pi、Claude Code 或 Codex 的特殊字段和完整兼容语义。

### Acceptance Signals

- Pi、Claude Code、Codex 可以分别通过各自 Adapter 路径发起请求，并使用各自声明的入站协议。
- 消费端只需要配置 `opus`、`fast` 或 `gpt-5.6-luna-1m` 等 Model ID；后台修改 Model 的渠道关联后，后续请求无需修改客户端配置即可切换。
- 首选渠道目标不可用时，请求可以按照 Model 关联的 priority 切换备用目标；成功响应中的模型仍是消费端请求的 Model ID。
- Model 的 ModelCard、渠道关联和 Adapter 绑定在服务重启后仍然有效；配置刷新不会短暂暴露空 Model 或部分旧配置。
- 未登录、无用户、无项目和无租户上下文的消费请求可以正常工作；错误的消费令牌不能访问管理接口。
- 管理端可以完成 Model 渠道目标切换，并能看到当前生效目标、健康状态和配置刷新结果。

## Implementation Contract

本节把上面的产品需求具体化为可执行的后端实现方案。实现顺序是“先新增适配器链路，再切换默认入口，最后裁剪旧平台能力”，避免一次性删除用户、项目和 API Key 表导致现有 AxonHub 运行链路不可恢复。

### Implementation Principles

1. **数据库是业务配置唯一来源**：Adapter、Model、ModelCard、ModelSettings.Associations 以及 Channel 均从 Ent 数据库读取；YAML/env 一期只保存服务基础设施配置，不新增消费 API Key 配置。
2. **运行时配置采用完整快照原子替换**：每次管理变更提交事务后重新构建完整路由快照；新快照未通过校验时继续使用旧快照，禁止发布半成品配置。
3. **适配器只改变入站协议和 Model 解析**：渠道凭证、出站 endpoint、协议转换、重试、熔断、限流和用量记录继续复用 AxonHub 现有链路。
4. **ModelCard 是能力与对外价格的单一来源**：同一个 Model 的上下文、输出限制、模态、工具/推理能力和对外价格统一配置；渠道实际成本单独读取 ChannelModelPrice，不反向改写 ModelCard。
4. **逻辑模型贯穿消费侧**：请求、响应、流式事件和模型列表使用 `sourceModelId`；只有结构化日志、运行时健康信息和请求执行记录使用 `targetModelId`。
5. **兼容性不靠猜测**：Pi、Claude Code、Codex 的真实请求、SSE、工具调用和错误样例在实现前落盘；没有样例的字段不进入适配器契约。
6. **保留旧数据表作为迁移兼容层**：第一阶段不物理删除 User、Project、APIKey、Role、OIDC 等 Ent schema，先从公网请求链路和默认后台入口解除依赖；后续确认迁移完成后再删除表和代码。

## Architecture

### Runtime Request Flow

```text
请求 /{adapter}/v1/{operation}
  -> AdapterConsumerInterceptor（一期读取但不校验消费层 API Key）
  -> AdapterRouteRegistry 按 adapter name/slug 读取不可变 AdapterSnapshot
  -> 校验 Adapter 只有一个 inbound APIFormat
  -> 适配器协议 handler（OpenAI Chat / OpenAI Responses / Anthropic Messages）
  -> 现有 inbound transformer
  -> AdapterCandidateSelector(modelID)
  -> Adapter 允许的 Model 绑定
  -> 读取 ModelCard 与 ModelSettings.Associations
  -> 按 association priority、请求条件和健康状态选择 Channel + physical model ID
  -> 校验 Channel 已配置对应 outbound endpoint
  -> 现有 channel outbound transformer + retry/circuit-breaker/rate-limit
  -> 逻辑 Model ID 响应恢复与适配器协议响应
```

`/{adapter}` 使用单个动态路由入口，由运行时 registry 校验数据库中的 Adapter；不为每条数据库配置重新注册 Gin 路由，因此修改 Adapter、绑定或模型组后不需要重启 HTTP server。Adapter 的 name/slug 就是公开 URL 路径，例如 `pi-anthropic` 对应 `/pi-anthropic/v1/`。同一个客户端可以创建多个 Adapter，但每个 Adapter 只能绑定一个入站 APIFormat。

### Adapter / Protocol Matrix

协议矩阵不再把 `pi`、`cc`、`codex` 写死为系统对象。以下只是由管理端创建的示例，最终 handler 仍由 Adapter.inbound APIFormat 决定：

| Adapter name/slug | 路径 | 入站协议 | 复用的 AxonHub handler | 典型入口 |
|---|---|---|---|---|
| `pi-anthropic` | `/pi-anthropic/v1/` | `anthropic_messages` | `AnthropicHandlers.ChatCompletionHandlers` | `/pi-anthropic/v1/messages` |
| `pi-openai` | `/pi-openai/v1/` | `openai_responses` 或 `openai_chat`（二选一，单个 Adapter 只能选一个） | 按配置选择对应 OpenAI handler | `/pi-openai/v1/responses` 或 `/pi-openai/v1/chat/completions` |
| `codex-openai` | `/codex-openai/v1/` | `openai_responses` | `OpenAIHandlers.ResponseCompletionHandlers` | `/codex-openai/v1/responses` |

Adapter 的 inbound APIFormat 决定允许的操作和协议错误格式。请求命中不匹配的操作时返回适配器协议对应的“unsupported endpoint”错误，而不是把请求降级到另一个协议。

### Data Model

本方案复用现有 Model 作为模型中心，不新增 ModelGroup、ModelGroupProtocol 或 ModelGroupTarget 作为第二套模型配置体系。

#### Adapter

- `name/slug`：唯一 URL path segment，同时就是公开消费入口名称，例如 `pi-anthropic`、`pi-openai`。
- `display_name`、`inbound_api_format`、`status`、`remark` 保持当前 Adapter MVP 语义。
- 一个 Adapter 只能声明一个入站协议。
- Adapter 只维护允许暴露的 Model ID，不维护渠道目标池。

#### Model

- 复用 `internal/ent/schema/model.go` 的全局唯一 `model_id` 作为对外逻辑模型 ID 和别名。
- `model_card` 是该 Model 的统一能力契约，包含 `Limit.Context`、`Limit.Output`、`Modalities`、`Vision`、`ToolCall`、`Reasoning` 和统一对外 `Cost`。
- 不同能力版本必须建立独立 Model，例如 `gpt-5.6-luna` 与 `gpt-5.6-luna-1m`；不新增 Alias 或 ModelVariant 表。
- `model.settings` 保存该 Model 的渠道关联、物理模型 ID、priority、启用状态和请求条件。
- 旧 Model 目录、ModelCard 和 ModelSettings.Associations 在迁移后成为 Adapter 路由的正式配置来源。

#### ModelSettings.Associations

- `channelModel` 关联表达 `channel_id + physical_model_id`；它是发送给渠道的真实模型目标。
- `priority` 数值越小越优先；相同 priority 继续复用现有 LoadBalancer 和健康状态处理。
- `when` 条件继续支持 prompt token、stream、request format、图片/视频/文档/音频和请求头等已有字段。
- 一个 Model 可以关联多个 Channel 和多个物理模型；只有能力符合该 ModelCard 的渠道目标才允许被配置到同一个 Model。
- Model 不复制 Channel 凭证；Channel 禁用或目标 endpoint 不可用时，目标由现有候选筛选和健康机制排除。

#### ChannelModelPrice

- 复用现有 `ChannelModelPrice` 保存各渠道物理模型的实际成本价格和 reference ID。
- ModelCard 的 `Cost` 保存统一对外价格；渠道实际价格只用于内部成本、用量和计费，不自动覆盖 ModelCard。

#### 迁移兼容

- 当前 AdapterModelBinding 的 `model_group_id` 不再作为长期模型关联；迁移目标是绑定现有 `Model.model_id`。
- 当前 ModelGroup、ModelGroupProtocol、ModelGroupTarget 数据先保留，迁移工具将其转换为 Model 的 ModelCard 和 ModelSettings.Associations。
- 迁移完成并验收后，停止 ModelGroup API、页面和运行时读取；物理表清理由单独迁移任务负责，不与首轮路由切换混合。

建议保留 `internal/objects/adapter.go` 只承载 Adapter 运行时快照和诊断 DTO；Model 能力继续使用现有 `objects.ModelCard`，避免形成第二套能力字段。
### Runtime Snapshot and Refresh

新增 `internal/server/biz/adapter.go`（或拆成 `adapter.go`、`model_group.go`）实现 `AdapterService`：

- `Refresh(ctx) (RefreshResult, error)`：查询所有 Adapter、允许的 Model、ModelCard、ModelSettings.Associations 和启用 Channel，构建完整 `AdapterSnapshot`。
- `Resolve(ctx, adapterName) (*RuntimeAdapter, error)`：只读访问当前快照，不在请求路径查询数据库。
- `ListModels(ctx, adapterName)`：返回该 Adapter 已启用且存在有效 Model 的 `model_id` 去重列表，按绑定定义稳定排序。
- `ReplaceAdapterConfig`、`ReplaceModelAssociations`、`ReplaceModelBindings`：分别在 `RunInTransaction` 中完成完整资源替换，提交成功后调用 `Refresh`。
- 使用 `atomic.Value` 或读写锁保存不可变快照；刷新时先在局部对象中完成所有校验，最后一次性 swap。
- 使用刷新互斥锁避免并发管理请求按旧写入顺序覆盖新快照；刷新失败只返回错误并保留旧快照。
- 记录单调递增 `snapshot_version`、`refreshed_at`、失败原因和最近一次成功刷新时间，供 `/admin/gateway/runtime` 展示。
- 服务启动时执行一次 Refresh；数据库配置无效时启动失败并明确打印 Adapter、绑定、目标和 Channel 的定位信息。启动后管理变更失败不得清空已生效快照。

目标校验顺序：Adapter inbound APIFormat -> Model 存在且启用 -> ModelCard 对外能力契约 -> ModelSettings.Associations 请求条件 -> Channel 可用状态 -> Channel endpoint 和 outbound 能力。ModelCard 不通过多个目标动态聚合；能力不一致的目标必须归入其他 Model ID/别名，校验失败的关联保留可定位诊断。

### Candidate Selection Integration

新增 `internal/server/orchestrator/adapter_selector.go`，实现现有 `CandidateSelector` 接口：

1. 从 request context 读取 `RuntimeAdapter`；缺失时返回内部配置错误，不回退到全渠道搜索。
2. 使用 `llm.Request.Model` 作为 Model ID 查找 Adapter 允许的 Model，并读取 ModelCard 与 ModelSettings.Associations。
3. 复用现有模型候选解析，把每条有效 association 转换为 `ChannelModelEntry{RequestModel: modelID, ActualModel: physicalModelID}`，保留 association priority 和请求条件。
4. 根据当前请求的入站格式、stream、tools、图片/音频/视频能力过滤不兼容关联；ModelCard 是统一契约，不在运行时取多个目标交集。
5. 让现有 `WithStreamPolicySelector`、quota selector、trace/thread selector、circuit-breaker 和 retry 链继续包裹该 selector；不修改 Channel outbound 选择逻辑。
6. 目标排序继续复用现有 association priority 和 LoadBalancer，保证优先目标始终排在备用目标之前。
7. 健康状态继续使用现有 `(channelID, physicalModelID)` 粒度，并由 Model/Adapter 运行时诊断暴露最近失败和切换原因；健康状态不写回 ModelCard。

需要同步修改：

- `internal/server/orchestrator/state.go`：补充 Adapter/Model 运行时元数据。
- `internal/server/orchestrator/select_candidates.go`：所有 `APIKey.GetActiveProfile()` 访问必须先判断 APIKey 是否为空；适配器请求不得执行项目/API Key Profile 的渠道过滤。
- `internal/server/orchestrator/model_mapper.go`：无 APIKey 时也必须保存 `OriginalModel`，并在响应和流式响应中将实际模型恢复为消费端请求的逻辑模型。
- `internal/server/orchestrator/orchestrator.go`：从 context 读取 Adapter 元数据并补充结构化日志字段；保留 `apiKey == nil` 的合法请求路径。
- `internal/server/orchestrator/request_execution.go`：日志中同时输出 adapter、model ID、channel、physical model、association priority 和 fallback reason；不把消费令牌写入日志。
- `internal/server/orchestrator/model_access.go`：适配器请求只做“模型非空”和 Adapter 允许的 Model 校验，不读取 API Key Profile。

### Protocol Handler Reuse

新增 `internal/server/api/adapter.go` 作为协议路由门面，不复制 OpenAI/Anthropic 的请求解析和流式写出逻辑：

- 注入 `*biz.AdapterService`、`*api.OpenAIHandlers`、`*api.AnthropicHandlers`。
- 通过现有 `ChatCompletionHandlers` 的 `WithStreamWriter` 和 `ChatCompletionWithRequest` 复用 transformer/orchestrator；为协议 handler 使用 `WithChannelSelector(adapterSelector)` 生成带适配器候选选择器的 orchestrator 副本。
- 暴露 `ChatCompletions`、`Responses`、`Messages`、`ListModels`、`RetrieveModel` 五类 handler；实际允许的 handler 由 Adapter protocol 决定。
- 不让 `/pi`、`/cc`、`/codex` 各自复制一份编排代码；差异仅在入站/出站协议 transformer 和错误/模型列表格式。
- 首批模型列表只返回该 Adapter 的逻辑模型 ID，不返回 target model、Channel 名称或 Channel 凭证信息。

## Route and Authentication Changes

### Routes

修改 `internal/server/routes.go`：

- 新增动态组 `/:adapter/v1`，挂载 request timeout、IP blocklist、`WithAdapterConsumerInterceptor`、`WithAdapterRoute`、`WithSource(request.SourceAPI)`、trace/thread 中间件。
- 注册：
  - `POST /:adapter/v1/chat/completions`
  - `POST /:adapter/v1/responses`
  - `POST /:adapter/v1/messages`
  - `GET /:adapter/v1/models`
  - `GET /:adapter/v1/models/:model`
- `WithAdapterRoute` 只允许当前快照中 enabled 的 Adapter；unknown/disabled adapter 返回 404，不泄露数据库详情。
- 旧 `/v1`、`/anthropic/v1`、Gemini 等通用消费入口增加 `legacy_api_enabled` 开关，默认关闭；迁移期可临时打开，但不能绕过 Adapter 路由和模型组配置。
- `/health` 保持免鉴权；OAuth、用户登录、JWT 管理入口不再作为默认公开路由。

### Consumer API Key Interceptor（MVP）

新增 `internal/server/middleware/adapter_consumer.go`：

- `WithAdapterConsumerInterceptor` 统一挂载到所有 `/:adapter/v1` 路由。
- 从 `Authorization: Bearer`、`x-api-key` 或 Anthropic `api-key` 头中提取消费层自己的 API Key（若存在），写入请求上下文供后续扩展使用。
- 一期默认不校验、不要求有效值、不落库、不记录明文、不转发到上游 Channel；上游请求始终使用 Channel 自己的凭证。
- 一期不新增 consumer token、admin token 或 APIKey 表绑定逻辑；管理接口复用当前项目管理鉴权，或在部署层限制为本机访问。
- 后续再增加 `consumer_auth.enabled`、全局 API Key 和 Adapter 级 API Key 校验；校验策略不进入一期 MVP。

一期只修改 `legacy_api_enabled` 等路由开关，不新增 token 配置。Model、ModelCard 和 Model 关联配置必须通过数据库管理 API 或现有 Model 管理入口刷新。

### Single-Instance Cut Boundary

第一阶段不直接删除 Ent 中的 User/Project/APIKey/Role/OIDC schema，而是切断公网请求依赖：

- 消费路由不调用 `AuthService.AuthenticateAPIKey`、`AuthenticateJWTToken`，不建立 APIKey、User、Project context。
- `/admin/gateway/*` 一期复用当前项目管理鉴权或本机访问约束；健康检查仍不需要消费 API Key。
- 删除/隐藏默认登录页、`/admin/auth/signin`、`/oauth/*`、用户/租户/项目/RBAC/API Key 管理菜单和对应前端调用。
- 保留 Channel 管理、Adapter 管理、Model 管理、Model 关联目标健康和刷新状态；渠道凭证仍由 Channel 管理能力维护。
- 旧 GraphQL resolver 和后台服务可以暂时保留为迁移兼容代码，但不能通过消费 token 访问，也不能重新成为 Adapter 路由的依赖。
- 迁移完成后再单独做 schema 清理任务，删除遗留表前先提供数据导出和回滚方案；本特性不把物理删表混入第一轮路由改造。

## Management API Contract

新增 `internal/server/api/gateway_admin.go`，所有接口挂在 `/admin/gateway`，一期复用当前项目管理鉴权或本机访问约束：

- `GET /adapters`：返回 Adapter、protocol、状态、绑定逻辑模型和快照版本。
- `PUT /adapters/:name`：整体替换 Adapter 基础配置和绑定列表；绑定校验失败时整笔回滚。
- `GET /models`：返回 Model、ModelCard、统一对外价格、渠道关联、priority、启用状态和诊断状态。
- `PUT /models/:model_id`：整体替换 ModelCard、统一对外价格和 ModelSettings.Associations；能力不一致的目标不得挂到同一个 Model。
- `GET /runtime`：返回 snapshot version、刷新时间、当前生效 Adapter、Model ID、渠道目标顺序、目标健康和最近切换原因。
- `POST /refresh`：显式从数据库重建快照；成功返回新版本，失败返回诊断且旧版本继续服务。

管理 API 的写请求使用 `RunInTransaction`，服务层执行以下约束：

- Adapter 名称和 Model ID 不得为空；Adapter 名称必须符合 path 规则。
- 一个 Adapter 下同一 Model ID 只能有一个 enabled binding；binding 必须指向 enabled Model。
- ModelCard 的能力和统一对外价格必须完整可解析；能力不同的别名必须使用独立 Model ID。
- Model association 必须引用 enabled Channel；physical model ID 必须非空。
- priority 采用低值优先，更新整个 Model 关联列表时禁止产生重复有效目标。
- 修改已被请求使用的配置不影响正在执行的 request；只影响 snapshot swap 之后的新请求。
- 数据库提交成功但快照刷新失败时，接口返回 500/409 诊断，旧快照继续服务，并提供下一次 `POST /refresh` 的入口；不得返回“刷新成功”。

错误响应统一分为：

- `invalid_adapter_request`：路径、JSON、协议或能力不符合 Adapter 契约。
- `model_unavailable`：Model 没有有效 Adapter binding 或没有可用渠道目标。
- `channel_unavailable`：目标 Channel 被禁用、删除或 outbound 初始化失败。
- `upstream_error`：目标渠道请求失败；由现有 retry/error transformer 继续处理。
- `gateway_config_error`：管理配置或快照刷新失败。

## File-Level Change Map

### 新增文件

- `internal/ent/schema/adapter.go`
- `internal/ent/schema/adapter_model_binding.go`（迁移期允许保留，长期改为 Model 绑定）
- `internal/objects/adapter.go`
- `internal/server/biz/adapter.go`（Adapter 快照、Model 绑定迁移、刷新、健康诊断）
- `internal/server/biz/model.go`（复用 ModelCard 与 ModelSettings.Associations 的管理和快照适配）
- `internal/server/orchestrator/adapter_selector.go`
- `internal/server/api/adapter.go`
- `internal/server/api/gateway_admin.go`
- `internal/server/middleware/adapter_consumer.go`
- `internal/contexts/adapter.go`
- `internal/server/api/testdata/adapter/`（真实协议 fixture，名称按实际客户端场景命名）
- `internal/server/api/adapter_test.go`
- `internal/server/api/gateway_admin_test.go`
- `internal/server/middleware/adapter_consumer_test.go`
- `internal/server/orchestrator/adapter_selector_test.go`
- `internal/server/biz/adapter_test.go`
- `integration_test/adapter/pi/`, `integration_test/adapter/cc/`, `integration_test/adapter/codex/`

### 修改文件

- `internal/server/routes.go`：动态 Adapter 路由、旧入口开关、管理认证边界。
- `internal/server/config.go`、`conf/conf.go`、`config.example.yml`：legacy 开关和一期拦截器基础配置；不新增消费 token 配置。
- `internal/server/biz/fx_module.go`、`internal/server/api/fx_module.go`：注册 AdapterService、Adapter handlers 和生命周期 Refresh。
- `internal/server/biz/channel.go`：增加按 enabled channel ID 读取运行时 Channel 的只读索引，供 selector 构造候选。
- `internal/server/orchestrator/state.go`、`orchestrator.go`、`select_candidates.go`、`model_mapper.go`、`model_access.go`、`request_execution.go`：接入 Adapter/Model 元数据、nil APIKey 路径、逻辑模型恢复和结构化观测。
- `internal/ent/schema/model.go`、`internal/objects/model.go`、`internal/ent/schema/channel_model_price.go`：补齐 ModelCard/统一对外价格与渠道实际价格的契约和查询边界。
- `internal/server/gql/*`：仅在迁移期保留渠道和系统管理所需字段；移除用户/项目/RBAC/API Key/OIDC 的公开操作及前端调用，不直接编辑生成的 `ent.graphql`。
- `frontend/src/*`：删除登录和租户/项目状态依赖；一期可继续复用现有后台管理鉴权；复用 Model 管理页面维护 ModelCard、统一价格、渠道关联和 priority，新增 Adapter 管理页面或先提供等价 REST 管理入口。
- `internal/ent/*`、`internal/server/gql/generated.go` 等生成文件：只通过 `make generate` 更新，不手工编辑。
- 迁移脚本/工具：读取现有 ModelGroup 配置并转换为 ModelCard 与 ModelSettings.Associations；具体路径在实现阶段依据当前数据迁移工具约定确定。

### 不应修改的成熟链路

- `internal/server/biz/channel_llm.go` 的渠道模型入口和 outbound endpoint 选择。
- `internal/server/orchestrator/outbound.go` 的 `ActualModel` 注入、retry candidate 切换和协议 outbound 转换。
- `llm/transformer/**` 的已有 OpenAI、Anthropic、Responses 转换器，除非真实 fixture 证明协议缺口。
- Channel 凭证、探活、限流、熔断、用量和请求执行持久化的既有实现；只增加 Adapter 观测字段。

## Implementation Phases

### Phase 0：协议样例和入口契约

1. 保留并补齐 Pi、Claude Code、Codex 的真实请求、SSE、工具调用和错误 fixture。
2. 在协议契约未冻结前，不扩展客户端专属字段，不声称完整兼容。

**完成信号**：请求侧和响应侧边界可由脱敏 fixture 重放，错误和流式模型恢复规则明确。

### Phase 1：Model 中心的数据契约

1. 确认现有 ModelCard 的能力、统一对外价格和 ModelSettings.Associations 的渠道关联语义。
2. 为 Model 管理补齐上下文、输出 Token、模态、工具/推理能力、统一价格和关联 priority 的读写契约。
3. 保持一个 Model ID 对应一套统一能力；能力不同的模型使用独立 Model ID。
4. 复用 ChannelModelPrice 作为渠道实际成本来源，不把渠道成本覆盖到 ModelCard。
5. 为旧 ModelGroup 配置设计可回滚的数据转换，确保迁移前后目标、priority 和协议边界可追踪。

**完成信号**：可以配置 `gpt-5.6-luna` 与 `gpt-5.6-luna-1m` 两个独立 Model，并分别保存能力、价格和渠道关联。

### Phase 2：Adapter 直接绑定 Model

1. 将 Adapter 的允许模型绑定从 `model_group_id` 调整为现有 `Model.model_id`。
2. Adapter 模型列表只返回 enabled 且存在有效配置的 Model ID。
3. 保留 Adapter 的入站协议边界，不让 Adapter 保存物理渠道或目标模型配置。
4. 迁移期同时读取旧 ModelGroup 配置并提供诊断，禁止新写入继续创建 ModelGroup 目标。

**完成信号**：同一个 Model 可以被多个 Adapter 复用；不同别名可以绑定不同 Adapter；消费端只看到 Model ID。

### Phase 3：复用 Model Associations 完成路由闭环

1. AdapterCandidateSelector 根据 Model ID 读取 ModelCard 和 ModelSettings.Associations。
2. 复用现有 ChannelModelEntry、association priority、请求条件、健康检查、quota、retry、circuit-breaker 和 failover。
3. 将 association 的 physical model ID 写入 `ActualModel`，将 Model ID保留在 `RequestModel` 和响应。
4. 不引入目标能力交集；能力契约以 ModelCard 为准，目标不符合时在配置校验或迁移诊断中暴露。
5. 补齐无 APIKey 时的模型访问、响应模型恢复和结构化日志。

**完成信号**：`gpt-5.6-luna` 先走 Kiro，Kiro 失败后按 Model association priority 切换备用渠道，消费端仍收到 `gpt-5.6-luna`。

### Phase 4：消费路由和管理入口收敛

1. 保留动态 `/{adapter}/v1` 路由和统一消费拦截器。
2. Adapter `/models` 返回 Adapter 允许的 Model ID 及对应 ModelCard 元数据。
3. 管理 API 从 ModelGroup CRUD 切换为 Model、ModelCard、统一价格和 Model associations CRUD。
4. 管理端复用现有 Model 页面维护能力和渠道关联；Adapter 页面只维护 Adapter 协议与 Model 绑定。
5. 配置提交后原子刷新 Model/Adapter 快照，失败时继续使用旧快照。

**完成信号**：后台修改 Model 的优先渠道或能力配置后，后续请求和 `/models` 返回立即使用新配置，无需重启客户端。

### Phase 5：ModelGroup 数据迁移和重复链路下线

1. 将已有 ModelGroup、Protocol、Target 和 Adapter binding 转换为 Model、ModelCard 和 ModelSettings.Associations。
2. 对无法一对一转换的能力、协议或目标输出明确迁移诊断，不静默合并。
3. 迁移验收通过后，停止 ModelGroup API、管理页面和运行时快照读取。
4. 保留旧表和回滚开关，先隐藏入口，再单独执行物理表清理。

**完成信号**：新部署和迁移部署都只以 Model 为模型中心，ModelGroup 不再参与新请求路由。

### Phase 6：单实例裁剪、真实验收和文档

1. 继续解除消费链路对 User、Project、APIKey、Role、OIDC 的依赖；保持一期消费 API Key 不校验。
2. 完成三类客户端的非流式、流式、工具调用、错误、模型列表和目标故障转移验收。
3. 更新 README、部署文档、Model/Adapter 管理说明和迁移回滚说明。
4. 暂不执行旧 Ent 表 drop，另建数据清理计划。

**完成信号**：新部署无需创建用户、项目或 API Key 即可配置 Model、Channel、Adapter，并完成真实上游请求。

## Verification Matrix

本轮只完成设计和计划，不运行构建、生成、测试或启动命令。进入实现并获得明确验证指令后，按以下顺序执行：

1. `git diff --check`：计划和代码无空白错误。
2. Ent schema/GraphQL 变更后执行 `make generate`，确认生成文件只来自生成器。
3. `internal/server/biz/model_test.go`、`internal/server/biz/adapter_test.go`：ModelCard/Associations 读写、统一能力契约、Model ID 别名、迁移回滚、快照原子替换和刷新失败保留旧版本。
4. `internal/server/orchestrator/adapter_selector_test.go`：Adapter 直接绑定 Model、Model associations priority、请求条件、physical model 注入、健康目标、无可用渠道错误和逻辑 Model ID 恢复。
5. `internal/server/middleware/adapter_consumer_test.go`：Authorization、x-api-key、Anthropic api-key 提取；无 API Key 默认放行；明文不进入日志和上游请求。
6. `internal/server/api/adapter_test.go`：各 Adapter 协议 fixture 的非流式、流式、工具调用、错误和包含 ModelCard 元数据的模型列表。
7. `integration_test/adapter/*`：通过真实 HTTP server 验证请求进入 Channel、priority 故障转移、响应模型恢复、Model 配置热刷新和 ModelGroup 迁移后的兼容性。
8. 前端类型检查和浏览器验收仅在 Model/Adapter 管理 UI 被纳入实现阶段后执行。

## Risks, Rollback, and Open Gates

- **真实协议不一致**：Phase 0 失败时不实现猜测字段；先补 fixture 或把对应 Adapter 标记 disabled。
- **旧 APIKey/Project 隐式依赖**：若测试暴露 Request、Prompt、DataStorage 或 GraphQL 仍要求 project anchor，先建立内部 compatibility anchor，不把它传入消费语义，再逐项拆依赖。
- **刷新竞态**：任何运行时对象都不能原地修改；使用“构建完整对象 -> 校验 -> 原子 swap”避免请求看到半配置。
- **目标健康粒度不足**：优先复用现有 `(channelID, physicalModelID)` 熔断状态；若实际链路只能得到 channel 级健康，必须在管理端明确显示粒度，不能声称是目标级健康。
- **ModelGroup 迁移不完整**：任何无法一对一转换为 ModelCard/ModelSettings.Associations 的配置都必须阻断该条迁移并报告原因，不能静默合并能力或优先级。
- **ModelCard 与渠道实际能力不一致**：ModelCard 是用户声明的统一契约；渠道目标不应被自动聚合到同一 Model，能力不同必须通过独立 Model ID 解决。
- **回滚**：`legacy_api_enabled=true` 可在迁移期恢复旧入口；Adapter/Model 快照保留上一版本，管理 API 刷新失败自动继续使用旧版本；数据库写入通过事务回滚。
- **未决门禁**：Pi/Claude Code/Codex 的真实协议 fixture、是否保留现有前端 UI、后续全局/Adapter 级消费 API Key 的启用和优先级策略。MVP 明确不校验消费 API Key。

## Plan Review State

- 原 MVP 计划的 ModelGroup 中心架构已按用户确认改为 Model 中心架构；Product Contract changed: R4-R15、R20-R25 的模型中心、能力契约、价格和迁移边界已同步调整。
- 已确认三个产品决策：Model 是唯一模型中心；不同能力的别名是独立 Model；ModelCard 统一对外价格，渠道价格用于内部成本/计费。
- 已基于当前 AxonHub 的 ModelCard、ModelSettings.Associations、ChannelService、orchestrator、Adapter 路由和数据库自动迁移入口完成整体方向修订。
- 当前 `ModelGroup` 能力配置提交属于迁移前的临时实现，不再继续扩展；迁移完成后下线其 API、页面和运行时读取。
- 已完成真实客户端请求侧 fixture 采集和脱敏，文件位于 `internal/server/api/testdata/adapter/`；该目录 README 明确记录了尚未采集的响应侧边界。
- 本次只更新计划，没有运行构建、测试或服务重启；进入实现前仍必须补齐真实上游成功 SSE、工具调用回合和错误响应。
