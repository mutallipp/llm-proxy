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

- **目标**：将 AxonHub 收敛为单实例、单用户、按客户端适配器接入的 AI 模型网关。消费层只使用稳定的逻辑模型 ID，后台通过模型组切换真实渠道和目标模型。
- **当前边界**：本计划覆盖 Pi、Claude Code、Codex 三类适配器，保留 AxonHub 的渠道、协议转换、健康检查、重试和故障转移能力；不覆盖企业多租户能力。
- **产品权威**：适配器选择模型组，模型组选择目标，不允许消费端依赖具体渠道或 `targetModelId`。
- **开放阻塞**：实现阶段仍需以实际客户端协议样例确认 Pi、Claude Code、Codex 的入站字段、流式事件和错误语义；这不改变当前的产品边界。

## Product Contract

### Summary

AxonHub 将从带登录和多租户的平台收敛为单实例适配器网关。每个适配器可以使用不同的入站协议，但统一暴露逻辑模型；逻辑模型通过可热加载的模型组路由到多个真实渠道目标，并按优先级自动故障转移。

### MVP Boundary

一期目标是基于当前 AxonHub 链路快速打通最小可用路径，而不是一次完成完整平台裁剪：

- 保留 User、Project、APIKey、Role、OIDC 等旧 Ent schema，先从 Adapter 消费请求链路解除依赖。
- 所有 Adapter 使用统一 `AdapterConsumerInterceptor`；消费层可携带自己的 API Key，但一期不校验、不落库、不转发、不强制存在。
- 不实现 global consumer API Key、Adapter 级 API Key、消费 token 和新的管理 token；管理接口复用当前管理鉴权或本机访问约束。
- 优先完成动态 Adapter 路由、固定入站协议、逻辑模型绑定、ModelGroupProtocol、目标优先级故障转移和热刷新。
- 暂不新增管理 UI，先提供现有管理入口或最小 REST 管理 API；不为兼容性以外的能力重写现有 Channel 编排链路。

### Problem Frame

当前 AxonHub 的请求入口以通用协议路径和 API Key 上下文为中心，模型映射分散在 API Key Profile、渠道设置和模型关联中。目标使用者需要让 Pi、Claude Code、Codex 等客户端稳定地使用 `fast`、`opus` 等逻辑模型，同时可以在后台切换 Kiro、Claude Code、转转站等真实目标，而不修改消费端配置，也不受用户、项目和租户模型约束。

### Key Decisions

- **Adapter 是公开入口实例，不是固定客户端类型**：（session-settled: user-approved — Adapter 名称就是消费层 URL 路径）。创建 Adapter=`pi-anthropic` 后，消费入口就是 `/pi-anthropic/v1/`；`pi`、`cc`、`codex` 只是命名或配置模板，不是写死的路由。
- **每个 Adapter 只绑定一个入站协议**：（session-settled: user-approved — 同一个客户端可以创建多个 Adapter，例如 Pi 分别创建 `pi-anthropic` 和 `pi-openai`，但单个入口的协议必须固定）。
- **Adapter 绑定 ModelGroup**：（session-settled: user-approved — 选择适配器到模型组而非适配器到具体模型，因为多个适配器应能复用同一组目标，消费端不应感知渠道）。
- **ModelGroup 可以支持多个入站协议**：（session-settled: user-approved — 同一逻辑模型组可以同时服务 Anthropic、OpenAI Chat 或 OpenAI Responses 入口；Adapter 绑定时只显示支持自身入站协议的模型组）。
- **ModelGroupTarget 显式绑定出站协议**：（session-settled: user-approved — 渠道可能同时有 Anthropic 和 OpenAI endpoint，且模型集合不同，所以目标必须选择具体 outbound APIFormat，不能完全依赖请求时自动挑 endpoint）。
- **ModelGroup 由多个 ModelGroupProtocol 组成，每个协议配置拥有自己的 ModelGroupTarget 目标池**：（session-settled: user-approved — 同一逻辑模型组可以服务多个入站协议，但不同协议可以使用不同渠道目标）。每个目标由渠道、真实 `targetModelId` 和出站 APIFormat 组成。
- **模型组默认采用优先级故障转移**：（session-settled: user-approved — 选择稳定的首选目标加健康感知的备用目标，而不是默认轮询，因为同名上游模型可能存在协议、能力、延迟和成本差异）。
- **业务配置继续持久化在数据库**：（session-settled: user-approved — 选择数据库作为 Adapter、ModelGroup、目标绑定和渠道配置的持久化来源，因为 AxonHub 的渠道和模型配置已经以 Ent/SQLite 为主）。YAML 只承担服务启动和基础设施配置，不作为业务模型映射的主存储。
- **一期消费入口默认不校验 API Key**：（session-settled: user-approved — 所有 Adapter 共用一个拦截器，读取消费层传入的 API Key 但一期不校验、不落库、不转发到上游；后续再增加全局和 Adapter 级 API Key 校验）。
- **保留简化管理能力**：（session-settled: user-approved — 保留单实例后台配置入口；一期复用当前管理鉴权或限制管理接口只允许本机访问，不为消费入口新增认证改造）。

### Existing Context

- 现有服务路由集中在 `internal/server/routes.go`，通用 API 入口挂载认证、请求来源、线程和 trace 中间件；现有 `/v1`、`/anthropic/v1`、Gemini 等协议入口并非按客户端适配器隔离。
- `internal/ent/schema/channel.go` 已持久化渠道类型、凭证、支持模型、渠道级设置和出站端点。
- `internal/objects/channel.go` 中的 `ChannelSettings.ModelMappings` 和 `internal/server/biz/channel_llm.go` 的 `GetModelEntries` 已支持单渠道模型别名到实际模型的转换，但它不表达可跨适配器复用的模型目标池。
- `internal/ent/schema/api_key.go` 和 `internal/server/orchestrator/model_mapper.go` 已支持 API Key Profile 级模型映射；该能力依赖 API Key、项目和用户上下文，不作为新的 ModelGroup 语义继续扩展。
- `internal/ent/schema/model.go` 的 `group` 字段是模型目录分类字段；`ModelSettings.Associations` 是渠道匹配规则，当前都不等价于可复用的模型组。
- `ChannelService` 已有启用渠道缓存和刷新机制，渠道运行时对象会预计算模型入口和出站转换器；新模型组配置需要拥有同等明确的更新生效边界。

### Requirements

#### Adapter 接入

- **R1**：系统必须按 Adapter 提供独立的对外 API 节点，使 Pi、Claude Code、Codex 可以使用不同的路径和入站协议。
- **R2**：每个 Adapter 必须声明唯一的入站协议和启用状态；所有 Adapter 路由统一经过消费拦截器。拦截器一期只提取消费层传入的 API Key，不校验、不落库、不转发；入站协议转换后的请求必须进入同一套模型组路由和渠道编排流程。
- **R3**：Adapter 暴露的模型列表和请求校验必须只呈现该 Adapter 允许使用的逻辑模型 ID，不得把渠道真实模型 ID 作为消费端的必需配置。

#### 逻辑模型组

- **R4**：系统必须提供可复用的 ModelGroup，作为 Adapter 与真实渠道目标之间的稳定边界。
- **R5**：Adapter 必须通过 `sourceModelId → ModelGroup` 绑定逻辑模型；同一 ModelGroup 可以被多个 Adapter 复用，不同 Adapter 也可以把同名 `sourceModelId` 绑定到不同 ModelGroup。
- **R6**：ModelGroupTarget 必须同时表达所属渠道、`targetModelId` 和明确的 outbound APIFormat，并支持启用状态、优先级以及用于判断目标可替代性的能力信息。
- **R7**：ModelGroup 可以声明多个支持的入站 APIFormat；Adapter 只能绑定一个入站 APIFormat，并且只能选择声明支持该协议的 ModelGroup。协议、工具调用、流式输出、上下文或模态能力不兼容的目标必须能够被隔离到不同协议配置或不同模型组。

#### 路由与故障转移

- **R8**：ModelGroup 默认必须按照目标优先级选择首选目标，并在目标发生可重试失败、超时、限流或熔断时切换到下一个可用目标。
- **R9**：目标健康状态、熔断状态和失败统计必须属于运行时状态，不得改变模型组的持久化语义；恢复后的目标应能重新参与选择。
- **R10**：模型组切换目标后，响应对消费端必须继续使用请求中的逻辑模型 ID；消费端不得依赖实际目标模型名。
- **R11**：渠道已有的出站协议转换、请求覆盖、流式转换和错误转换能力必须继续适用于模型组目标；ModelGroup 不得绕过 Channel 的出站适配能力。

#### 数据持久化与配置生效

- **R12**：Adapter、AdapterModelBinding、ModelGroup、ModelGroupProtocol、ModelGroupTarget 和 Channel 的业务配置必须持久化在数据库，并在服务重启后恢复。
- **R13**：管理端修改模型组目标、目标优先级、绑定关系或适配器协议后，运行时必须能够刷新相关缓存并让后续请求使用新配置，不要求消费端或 Agent 重启。
- **R14**：配置刷新必须保持一致性：单次变更不得产生部分 Adapter 绑定、部分目标列表或旧新配置混用的可观察状态。
- **R15**：配置更新失败时，系统必须保留上一次可用的运行时配置，并向管理端返回可定位的错误；不得静默切换到空模型组。

#### 单实例边界与鉴权

- **R16**：消费请求不得依赖用户登录、JWT 用户上下文、项目上下文、租户上下文、角色权限或用户级 API Key。
- **R17**：一期不实现消费 API Key 校验；统一拦截器允许消费层携带自己的 API Key，但默认不要求有效值。后续可增加全局 API Key 和 Adapter 级 API Key 校验开关，不重新引入用户级密钥管理。
- **R18**：一期管理接口继续复用当前项目管理鉴权或本机访问约束；不把消费层 API Key 当作管理凭证。
- **R19**：即使一期消费入口不校验 API Key，也必须在路由和代码层明确区分消费接口、管理接口和健康检查接口。

#### 简化管理能力

- **R20**：管理端必须能够维护 Adapter、逻辑模型、ModelGroup、ModelGroupProtocol、ModelGroupTarget、优先级、启用状态和渠道绑定。
- **R21**：管理端必须能够展示一个逻辑模型当前绑定的目标顺序、目标健康状态和最近一次切换原因，帮助使用者确认模型是否已切换。
- **R22**：管理端必须提供显式配置刷新或等价的自动刷新反馈，使使用者可以确认数据库配置已经进入运行时。
- **R23**：管理端不再提供用户、租户、项目、角色、OIDC 登录、个人 API Key 和企业权限管理功能。

#### 兼容与可观测性

- **R24**：每次请求的日志和追踪信息必须同时记录 Adapter、逻辑模型 ID、ModelGroup、实际渠道和 `targetModelId`，但响应和消费端可见模型必须保持逻辑模型 ID。
- **R25**：请求失败时，错误信息必须区分入站协议错误、模型组无可用目标、目标渠道失败和配置刷新失败，便于判断是客户端、路由还是上游问题。
- **R26**：Pi、Claude Code、Codex 的首批适配器必须分别具备代表性请求、流式请求、工具调用和错误场景的兼容性验收样例；具体字段以实际客户端协议为准。

### Core Flow

Adapter 是由管理端创建的公开消费入口，不是固定的 `pi`、`cc`、`codex` 三类系统对象。Adapter 的 `name/slug` 同时决定 URL 路径，例如 Adapter=`pi-anthropic` 对应 `/pi-anthropic/v1/...`。每个 Adapter 只能声明一个入站 APIFormat；同一个客户端可以创建多个 Adapter，例如 Pi 同时创建 `pi-anthropic` 和 `pi-openai`。

```mermaid
flowchart TD
    A[客户端请求] --> B[/{adapter}/v1 路由]
    B --> C[按 adapter name/slug 查 Adapter]
    C --> D[校验唯一入站 APIFormat]
    D --> E[入站协议转换]
    E --> F[sourceModelId]
    F --> G[AdapterModelBinding]
    G --> H[校验 ModelGroup 支持该入站 APIFormat]
    H --> I[按优先级与健康状态选择 Target]
    I --> J[ModelGroupTarget: Channel + targetModelId + outbound APIFormat]
    J --> K[选择 Channel 对应 endpoint]
    K --> L[出站协议转换]
    L --> M[上游渠道]
    M --> N[响应恢复为逻辑模型 ID]
```

ModelGroup 可以同时声明多个入站 APIFormat。ModelGroupTarget 必须明确选择 Channel 的一个 outbound APIFormat，因为同一个 Channel 可以同时提供 Anthropic、OpenAI Chat 或 OpenAI Responses endpoint，且不同 endpoint 的模型集合可能不同。

### Out of Scope

- 多用户、多租户、项目隔离、RBAC、OIDC 和用户登录流程。
- 用户级 API Key 生命周期、用户级配额和按租户计费策略。
- 让消费端直接选择或依赖 `targetModelId`。
- 默认对不同能力或不同协议目标进行无条件轮询。
- 重写 AxonHub 已有的渠道凭证、出站协议转换、流式处理、重试、熔断和请求记录能力。
- 在没有真实客户端协议样例前，臆造 Pi、Claude Code 或 Codex 的特殊字段和完整兼容语义。

### Acceptance Signals

- Pi、Claude Code、Codex 可以分别通过各自 Adapter 路径发起请求，并使用各自声明的入站协议。
- 消费端只需要配置 `opus`、`fast` 等逻辑模型 ID；后台修改 ModelGroup 目标后，后续请求无需修改客户端配置即可切换。
- 首选目标不可用时，请求可以按照优先级切换备用目标；成功响应中的模型仍是消费端请求的逻辑模型 ID。
- 数据库中的 Adapter、ModelGroup 和目标绑定在服务重启后仍然有效；配置刷新不会短暂暴露空路由或部分旧配置。
- 未登录、无用户、无项目和无租户上下文的消费请求可以正常工作；错误的消费令牌不能访问管理接口。
- 管理端可以完成模型组目标切换，并能看到当前生效目标、健康状态和配置刷新结果。

## Implementation Contract

本节把上面的产品需求具体化为可执行的后端实现方案。实现顺序是“先新增适配器链路，再切换默认入口，最后裁剪旧平台能力”，避免一次性删除用户、项目和 API Key 表导致现有 AxonHub 运行链路不可恢复。

### Implementation Principles

1. **数据库是业务配置唯一来源**：Adapter、AdapterModelBinding、ModelGroup、ModelGroupProtocol、ModelGroupTarget 以及 Channel 均从 Ent 数据库读取；YAML/env 一期只保存服务基础设施配置，不新增消费 API Key 配置。
2. **运行时配置采用完整快照原子替换**：每次管理变更提交事务后重新构建完整路由快照；新快照未通过校验时继续使用旧快照，禁止发布半成品配置。
3. **适配器只改变入站协议和逻辑模型解析**：渠道凭证、出站 endpoint、协议转换、重试、熔断、限流和用量记录继续复用 AxonHub 现有链路。
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
  -> AdapterCandidateSelector(sourceModelId)
  -> AdapterModelBinding(sourceModelId -> ModelGroup)
  -> 解析 ModelGroupProtocol(inbound APIFormat) 及其目标池
  -> ModelGroupTarget(priority, channel, targetModelId, outbound APIFormat)
  -> 校验 Channel 已配置对应 outbound endpoint
  -> 现有 channel outbound transformer + retry/circuit-breaker/rate-limit
  -> 逻辑模型响应恢复与适配器协议响应
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

新增五个 Ent schema。它们不复用 `Model.group`、`ModelSettings.Associations` 或 API Key Profile 映射，因为那些模型分别表达目录分类、渠道匹配规则和用户级映射。

#### Adapter

- `name/slug`：唯一 URL path segment，同时就是公开消费入口名称，例如 `pi-anthropic`、`pi-openai`；只允许小写字母、数字、`-`、`_`。
- `display_name`：后台展示名称。
- `inbound_api_format`：唯一入站协议，例如 `openai_chat`、`openai_responses`、`anthropic_messages`；一个 Adapter 只能有一个值。
- `status`：`enabled`、`disabled`、`archived`。
- `remark`：管理备注。
- 使用 `TimeMixin` 和软删除；对 `(name, deleted_at)` 建唯一索引。
- 关系：`Adapter -> AdapterModelBinding` 一对多。

#### AdapterModelBinding

- `adapter_id`：所属 Adapter。
- `source_model_id`：消费端看到的逻辑模型 ID，例如 `fast`、`opus`。
- `model_group_id`：目标模型组；禁止直接存 `channel_id` 或 `target_model_id`。
- `enabled`：是否对该 Adapter 暴露。
- `remark`：管理备注。
- 对 `(adapter_id, source_model_id, deleted_at)` 建唯一索引。
- 绑定必须指向启用的 ModelGroup；同一 ModelGroup 可以被多个 Adapter 复用。

#### ModelGroup

- `name`：唯一内部名称，例如 `fast-default`。
- `display_name`、`remark`。
- `status`：`enabled`、`disabled`、`archived`。
- `selection_strategy`：首版只实现 `priority_failover`，为后续扩展保留枚举。
- 使用 `TimeMixin` 和软删除；对 `(name, deleted_at)` 建唯一索引。
- 关系：`ModelGroup -> AdapterModelBinding`、`ModelGroup -> ModelGroupProtocol`。
- ModelGroup 不是单协议对象；同一个组可以通过多个 ModelGroupProtocol 声明支持多个入站协议。

#### ModelGroupProtocol

- `model_group_id`：所属模型组。
- `inbound_api_format`：该模型组协议配置允许被哪些 Adapter 入站协议使用，例如 `anthropic_messages`、`openai_responses`。
- `enabled`、`remark`。
- 该记录是一个协议级路由配置，拥有自己的 ModelGroupTarget 目标池；这样同一个 ModelGroup 可以让 Anthropic Adapter 使用 Kiro Anthropic 目标，让 OpenAI Adapter 使用 OpenAI Responses 目标，而不需要复制整个 ModelGroup。
- 对 `(model_group_id, inbound_api_format, deleted_at)` 建唯一索引。
- Adapter 绑定模型组时，只展示存在对应启用记录的 ModelGroup。

#### ModelGroupTarget

- `model_group_protocol_id`：所属 ModelGroupProtocol；因此目标池天然按入站协议隔离。
- `channel_id`：实际渠道。
- `target_model_id`：发送给渠道的真实模型 ID，例如 `gpt-5.6`。
- `outbound_api_format`：明确选择该 Channel 的一个出站 endpoint APIFormat；例如 `anthropic_messages` 或 `openai_responses`。
- `priority`：整数，数值越小优先级越高；相同优先级按 Ent ID 稳定排序。
- `enabled`：是否参与选择。
- `capabilities`：JSON，首版包含 `supports_tools`、`supports_stream`、`input_modalities`、`output_modalities`；缺省值表示沿用 Channel 能力探测结果。
- `remark`：管理备注。
- 对 `(model_group_protocol_id, channel_id, target_model_id, outbound_api_format, deleted_at)` 建唯一索引。
- 目标不复制 Channel 凭证；删除或禁用 Channel、或 Channel 不再提供对应 outbound APIFormat 后，目标在快照构建时自动变为不可用并在后台报告原因。

建议新增 `internal/objects/adapter.go` 定义能力 JSON 和运行时 DTO；协议和状态优先使用 Ent enum，避免在管理 API 中散落字符串常量。

### Runtime Snapshot and Refresh

新增 `internal/server/biz/adapter.go`（或拆成 `adapter.go`、`model_group.go`）实现 `AdapterService`：

- `Refresh(ctx) (RefreshResult, error)`：查询所有 Adapter、绑定、ModelGroup、ModelGroupProtocol、目标和启用 Channel，构建完整 `AdapterSnapshot`。
- `Resolve(ctx, adapterName) (*RuntimeAdapter, error)`：只读访问当前快照，不在请求路径查询数据库。
- `ListModels(ctx, adapterName)`：返回该 Adapter 已启用且绑定有效的 `source_model_id` 去重列表，按绑定定义稳定排序。
- `ReplaceAdapterConfig`、`ReplaceModelGroupTargets`、`ReplaceBindings`：分别在 `RunInTransaction` 中完成完整资源替换，提交成功后调用 `Refresh`。
- 使用 `atomic.Value` 或读写锁保存不可变快照；刷新时先在局部对象中完成所有校验，最后一次性 swap。
- 使用刷新互斥锁避免并发管理请求按旧写入顺序覆盖新快照；刷新失败只返回错误并保留旧快照。
- 记录单调递增 `snapshot_version`、`refreshed_at`、失败原因和最近一次成功刷新时间，供 `/admin/gateway/runtime` 展示。
- 服务启动时执行一次 Refresh；数据库配置无效时启动失败并明确打印 Adapter、绑定、目标和 Channel 的定位信息。启动后管理变更失败不得清空已生效快照。

目标能力校验顺序：Adapter inbound APIFormat -> ModelGroupProtocol 声明 -> 入站请求能力 -> Channel 可用状态 -> ModelGroupTarget.outbound_api_format 对应的 Channel endpoint -> outbound 能力 -> target capabilities。校验失败的目标不进入可用候选，但必须在管理端的诊断结果中保留原因。

### Candidate Selection Integration

新增 `internal/server/orchestrator/adapter_selector.go`，实现现有 `CandidateSelector` 接口：

1. 从 request context 读取 `RuntimeAdapter`；缺失时返回内部配置错误，不回退到全渠道搜索。
2. 使用 `llm.Request.Model` 作为 `sourceModelId` 查找 AdapterModelBinding，并校验 ModelGroup 存在启用的同入站 APIFormat 的 ModelGroupProtocol。
3. 只读取该 ModelGroupProtocol 自己的目标池，按 ModelGroupTarget 的 `priority ASC, id ASC` 构造 `ChannelModelsCandidate`；每个目标生成一个 `ChannelModelEntry{RequestModel: sourceModelId, ActualModel: targetModelId}`，并携带目标明确指定的 `outbound_api_format`。
4. 根据当前请求的 inbound APIFormat、目标 outbound APIFormat、stream、tools、图片/音频/视频能力过滤不兼容目标。
5. 让现有 `WithStreamPolicySelector`、quota selector、trace/thread selector、circuit-breaker 和 retry 链继续包裹该 selector；不修改 Channel outbound 选择逻辑。
6. 目标排序使用不同 priority 分组，保证优先目标始终排在备用目标之前；现有 LoadBalancer 只在同优先级目标内部工作。
7. 目标的实际健康状态复用现有 `ModelCircuitBreaker` 的 `(channelID, targetModelId)` 键，并由 AdapterService 维护面向管理端的最近失败/切换原因缓存。健康缓存是运行时状态，不写回 ModelGroupTarget 的持久化配置。

需要同步修改：

- `internal/server/orchestrator/state.go`：补充 Adapter/ModelGroup 运行时元数据。
- `internal/server/orchestrator/select_candidates.go`：所有 `APIKey.GetActiveProfile()` 访问必须先判断 APIKey 是否为空；适配器请求不得执行项目/API Key Profile 的渠道过滤。
- `internal/server/orchestrator/model_mapper.go`：无 APIKey 时也必须保存 `OriginalModel`，并在响应和流式响应中将实际模型恢复为消费端请求的逻辑模型。
- `internal/server/orchestrator/orchestrator.go`：从 context 读取 Adapter 元数据并补充结构化日志字段；保留 `apiKey == nil` 的合法请求路径。
- `internal/server/orchestrator/request_execution.go`：日志中同时输出 adapter、source model、model group、channel、target model 和 fallback reason；不把消费令牌写入日志。
- `internal/server/orchestrator/model_access.go`：适配器请求只做“模型非空”和 Adapter binding 校验，不读取 API Key Profile。

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

一期只修改 `legacy_api_enabled` 等路由开关，不新增 token 配置。模型组业务配置必须通过数据库管理 API 或现有管理入口刷新。

### Single-Instance Cut Boundary

第一阶段不直接删除 Ent 中的 User/Project/APIKey/Role/OIDC schema，而是切断公网请求依赖：

- 消费路由不调用 `AuthService.AuthenticateAPIKey`、`AuthenticateJWTToken`，不建立 APIKey、User、Project context。
- `/admin/gateway/*` 一期复用当前项目管理鉴权或本机访问约束；健康检查仍不需要消费 API Key。
- 删除/隐藏默认登录页、`/admin/auth/signin`、`/oauth/*`、用户/租户/项目/RBAC/API Key 管理菜单和对应前端调用。
- 保留 Channel 管理、Adapter 管理、ModelGroup 管理、目标健康和刷新状态；渠道凭证仍由 Channel 管理能力维护。
- 旧 GraphQL resolver 和后台服务可以暂时保留为迁移兼容代码，但不能通过消费 token 访问，也不能重新成为 Adapter 路由的依赖。
- 迁移完成后再单独做 schema 清理任务，删除遗留表前先提供数据导出和回滚方案；本特性不把物理删表混入第一轮路由改造。

## Management API Contract

新增 `internal/server/api/gateway_admin.go`，所有接口挂在 `/admin/gateway`，一期复用当前项目管理鉴权或本机访问约束：

- `GET /adapters`：返回 Adapter、protocol、状态、绑定逻辑模型和快照版本。
- `PUT /adapters/:name`：整体替换 Adapter 基础配置和绑定列表；绑定校验失败时整笔回滚。
- `GET /model-groups`：返回模型组及每个目标的 priority、Channel、targetModelId、启用状态和诊断状态。
- `PUT /model-groups/:name`：整体替换模型组目标列表；支持在一次请求内调整目标、优先级、targetModelId 和能力声明。
- `GET /runtime`：返回 snapshot version、刷新时间、当前生效 Adapter、逻辑模型、目标顺序、目标健康和最近切换原因。
- `POST /refresh`：显式从数据库重建快照；成功返回新版本，失败返回诊断且旧版本继续服务。

管理 API 的写请求使用 `RunInTransaction`，服务层执行以下约束：

- Adapter 名称和 source model ID 不得为空；名称必须符合 path 规则。
- 一个 Adapter 下同一 source model 只能有一个 enabled binding。
- enabled binding 必须指向 enabled ModelGroup；ModelGroup 至少有一个 enabled target。
- enabled target 必须引用 enabled Channel；targetModelId 必须非空。
- priority 采用低值优先，更新整个目标列表时禁止产生重复有效目标。
- 修改已被请求使用的配置不影响正在执行的 request；只影响 snapshot swap 之后的新请求。
- 数据库提交成功但快照刷新失败时，接口返回 500/409 诊断，旧快照继续服务，并提供下一次 `POST /refresh` 的入口；不得返回“刷新成功”。

错误响应统一分为：

- `invalid_adapter_request`：路径、JSON、协议或能力不符合 Adapter 契约。
- `model_group_unavailable`：逻辑模型没有有效 binding 或没有可用目标。
- `channel_unavailable`：目标 Channel 被禁用、删除或 outbound 初始化失败。
- `upstream_error`：目标渠道请求失败；由现有 retry/error transformer 继续处理。
- `gateway_config_error`：管理配置或快照刷新失败。

## File-Level Change Map

### 新增文件

- `internal/ent/schema/adapter.go`
- `internal/ent/schema/adapter_model_binding.go`
- `internal/ent/schema/model_group.go`
- `internal/ent/schema/model_group_protocol.go`
- `internal/ent/schema/model_group_target.go`
- `internal/objects/adapter.go`
- `internal/server/biz/adapter.go`（快照、CRUD、刷新、健康诊断）
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
- `internal/server/orchestrator/state.go`、`orchestrator.go`、`select_candidates.go`、`model_mapper.go`、`model_access.go`、`request_execution.go`：接入 Adapter 元数据、nil APIKey 路径、逻辑模型恢复和结构化观测。
- `internal/server/gql/*`：仅在迁移期保留渠道和系统管理所需字段；移除用户/项目/RBAC/API Key/OIDC 的公开操作及前端调用，不直接编辑生成的 `ent.graphql`。
- `frontend/src/*`：删除登录和租户/项目状态依赖；一期可继续复用现有后台管理鉴权；新增 Adapter/ModelGroup/target 管理页面或先提供等价 REST 管理入口。
- `internal/ent/*`、`internal/server/gql/generated.go` 等生成文件：只通过 `make generate` 更新，不手工编辑。

### 不应修改的成熟链路

- `internal/server/biz/channel_llm.go` 的渠道模型入口和 outbound endpoint 选择。
- `internal/server/orchestrator/outbound.go` 的 `ActualModel` 注入、retry candidate 切换和协议 outbound 转换。
- `llm/transformer/**` 的已有 OpenAI、Anthropic、Responses 转换器，除非真实 fixture 证明协议缺口。
- Channel 凭证、探活、限流、熔断、用量和请求执行持久化的既有实现；只增加 Adapter 观测字段。

## Implementation Phases

### Phase 0：协议样例和入口契约

1. 已从本机实际 Pi、Claude Code、Codex 客户端采集最小请求、流式标志、工具定义和关键入站 headers；捕获过程只访问 loopback 临时 HTTP 服务。
2. 已将敏感 token、真实 prompt、账号信息、动态会话/请求/安装 ID 脱敏后保存到 `internal/server/api/testdata/adapter/`，并在 README 中记录样例来源和边界。
3. 已确认 `pi -> openai_responses`、`cc -> anthropic_messages`、`codex -> openai_responses`；Pi 和 Codex 虽共用 Responses 请求体，仍必须使用独立 Adapter 路径和独立客户端 headers。
4. 待补充真实上游成功 SSE、工具调用回合和错误响应，再冻结完整协议矩阵：入站 headers、model 字段、stream 事件、tool call 事件、错误 body、模型列表要求、trace header。

**当前状态**：请求侧 fixture 已完成；响应侧 fixture 未完成，Phase 0 尚未通过实施门禁。

### Phase 1：Ent 数据模型和管理服务骨架

1. 新增四个 schema、objects DTO、索引和边关系。
2. 通过 `make generate` 生成 Ent client 和必要 GraphQL 代码；不写手工迁移 SQL。
3. 实现 AdapterService 的查询、字段校验、完整快照构建和原子刷新。
4. 增加 ChannelService 的 enabled channel ID 索引，处理 channel cache swap 时同步更新。
5. 加入 FX provider 和启动 Refresh hook。

**完成信号**：内存 Ent 测试可以创建 Adapter -> Binding -> Group -> Target -> Channel，并验证非法引用不会替换旧快照。

### Phase 2：适配器候选选择和逻辑模型闭环

1. 实现 AdapterCandidateSelector，把逻辑模型映射为候选的 `RequestModel`，把 targetModelId 写入 `ActualModel`。
2. 接入现有能力过滤、quota、health、retry、circuit-breaker 和 failover。
3. 修复无 APIKey 时 `OriginalModel`、响应模型和流式模型字段恢复。
4. 让请求执行日志同时记录 Adapter、source model、group、channel、target model。
5. 为 OpenAI Chat、Anthropic Messages、OpenAI Responses 分别创建带 Adapter selector 的 handler 副本。

**完成信号**：不走 HTTP 的 orchestrator 测试可以证明 `fast -> gpt-5.6`，首选失败后按 priority 选择备用目标，消费端响应模型仍为 `fast`。

### Phase 3：消费路由和统一拦截器

1. 实现统一 `AdapterConsumerInterceptor`，提取 Authorization、x-api-key 和 Anthropic api-key 头；一期默认放行，不调用用户/APIKey/JWT 认证链。
2. 注册动态 `/{adapter}/v1` 路由，按 Adapter protocol 分发操作。
3. 加入 Adapter model list，只返回逻辑模型。
4. 默认关闭旧 `/v1` 和 `/anthropic/v1` 公网入口；保留显式 legacy 开关用于迁移。
5. 保持 `/health` 免消费 API Key，并补充 unknown adapter、缺少 API Key 仍放行、明文不进入日志/上游的用例。

**完成信号**：多个 Adapter 路径可以被不同协议客户端调用，客户端无需知道渠道和 targetModelId；消费层 API Key 一期不校验。

### Phase 4：管理 API 和热刷新

1. 实现 Adapter、Binding、ModelGroup、Target 的整体替换 API。
2. 每次成功提交后刷新完整 snapshot；暴露 version、诊断、健康和最近 fallback reason。
3. 添加管理端显式 refresh；刷新失败保留旧配置并返回可定位错误。
4. 保留现有 Channel 管理能力作为凭证/渠道维护入口，移除其用户/租户选择语义。
5. 若项目需要 UI，再在此阶段接入最小 Adapter/ModelGroup 管理页面；否则以 REST 管理 API 和 curl 示例作为第一版后台。

**完成信号**：后台把 `fast` 的首选目标从 `gpt-5.6` 改为另一个 targetModelId 后，下一次请求立即使用新目标，客户端配置和 Agent 进程均不变。

### Phase 5：登录、多租户和旧入口裁剪

1. 移除或隐藏前端登录、JWT refresh、OIDC、项目切换、用户级 API Key 和 RBAC 菜单/调用；一期允许保留内部兼容代码。
2. 消费入口只保留 Adapter 路由；管理入口继续使用当前项目管理鉴权或本机访问约束。
3. 删除不再暴露的 GraphQL 操作和 resolver 依赖；保留内部遗留 Ent schema 直到完成数据迁移。
4. 更新 README、配置示例和部署文档，明确一期消费 API Key 不校验，以及后续全局/Adapter 级校验的预留方向。
5. 单独记录旧数据表清理作为后续迁移任务，不在此阶段执行 drop table。

**完成信号**：新部署不需要创建用户、项目或 API Key 即可配置渠道、模型组和 Adapter，并能完成一次真实上游请求。

## Verification Matrix

本轮只完成设计和计划，不运行构建、生成、测试或启动命令。进入实现并获得明确验证指令后，按以下顺序执行：

1. `git diff --check`：计划和代码无空白错误。
2. Ent schema/GraphQL 变更后执行 `make generate`，确认生成文件只来自生成器。
3. `internal/server/biz/adapter_test.go`：事务回滚、唯一约束、快照原子替换、刷新失败保留旧版本。
4. `internal/server/orchestrator/adapter_selector_test.go`：Adapter 单入站协议、ModelGroup 多入站协议过滤、target outbound APIFormat、逻辑模型绑定、优先级、能力过滤、健康目标、无可用目标错误。
5. `internal/server/middleware/adapter_consumer_test.go`：Authorization、x-api-key、Anthropic api-key 提取；无 API Key 默认放行；明文不进入日志和上游请求。
6. `internal/server/api/adapter_test.go`：各 Adapter 协议 fixture 的非流式、流式、工具调用、错误和模型列表。
7. `integration_test/adapter/*`：通过真实 HTTP server 验证请求进入 Channel、失败转移、响应模型恢复和热刷新。
8. 前端类型检查和浏览器验收仅在 UI 被纳入实现范围后执行。

## Risks, Rollback, and Open Gates

- **真实协议不一致**：Phase 0 失败时不实现猜测字段；先补 fixture 或把对应 Adapter 标记 disabled。
- **旧 APIKey/Project 隐式依赖**：若测试暴露 Request、Prompt、DataStorage 或 GraphQL 仍要求 project anchor，先建立内部 compatibility anchor，不把它传入消费语义，再逐项拆依赖。
- **刷新竞态**：任何运行时对象都不能原地修改；使用“构建完整对象 -> 校验 -> 原子 swap”避免请求看到半配置。
- **目标健康粒度不足**：优先复用现有 `(channelID, targetModelId)` 熔断状态；若实际链路只能得到 channel 级健康，必须在管理端明确显示粒度，不能声称是 target 级健康。
- **回滚**：`legacy_api_enabled=true` 可在迁移期恢复旧入口；Adapter 快照保留上一版本，管理 API 刷新失败自动继续使用旧版本；数据库写入通过事务回滚。
- **未决门禁**：Pi/Claude Code/Codex 的真实协议 fixture、是否保留现有前端 UI、后续全局/Adapter 级消费 API Key 的启用和优先级策略。MVP 明确不校验消费 API Key。

## Plan Review State

- 已基于当前 AxonHub 路由、Ent schema、ChannelService、orchestrator、认证中间件和数据库自动迁移入口完成文件级拆解。
- 已完成真实客户端请求侧 fixture 采集和脱敏，文件位于 `internal/server/api/testdata/adapter/`；该目录 README 明确记录了尚未采集的响应侧边界。
- 未执行构建、测试或服务重启，符合当前仓库规则和本次阶段边界。
- 进入实现前仍必须补齐真实上游成功 SSE、工具调用回合和错误响应；在此之前只能实现数据模型、快照和协议无关的路由骨架，不能声称 Pi/Claude Code/Codex 完整兼容。
