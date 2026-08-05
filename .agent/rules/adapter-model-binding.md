---
alwaysApply: false
globs: "frontend/src/features/adapters/**/*.ts, frontend/src/features/channels/**/*.ts, frontend/src/features/models/**/*.ts, frontend/src/lib/adapterApi.ts, internal/objects/model.go, internal/objects/adapter.go, internal/objects/channel.go, internal/server/orchestrator/**/*.go, internal/server/biz/adapter*.go, internal/server/biz/channel_capability*.go, internal/server/biz/protocol_pool_derivation*.go, internal/server/biz/model_capability_validation*.go, internal/server/api/adapter*.go, internal/server/api/gateway.go"
---

# Adapter / Model 绑定规则

## 渠道协议能力声明与自动派生（2026-08-05）

1. 协议×模型能力是渠道层客观事实，由渠道 `protocol_capabilities`（`DeclaredProtocols` + 逐模型 `Protocols`）声明；唯一写入面是 `saveChannelCapabilities`（含端点能力校验与协议族白名单校验，`Create/UpdateChannelInput` 不含该字段）。继承与派生触发只发生在能力表写入面（能力保存、auto_sync 联动）；UpdateChannel/批量导入写 supported_models 不触发继承。
2. 保存能力后由 `protocol_pool_derivation.go` 对账，为同名逻辑模型物化 `settings.protocolPools` 的 **auto 派生条目**（默认禁用、带 `auto`/`disabledReason`）。对账只碰 auto 条目；手动条目（含存量显式关联与 developer 继承层）永不被派生逻辑修改或删除。
3. auto 条目一经启用即转手动，脱离对账处置；auto 条目不可删除、只可禁用，服务端拒绝删除现存 auto 条目。启用 auto 条目（首次/批量/撤销后重启用）统一走 `model_capability_validation.go` 的校验（渠道启用、声明存在、端点族、物理模型条目可解析），钩子覆盖 CreateModel/UpdateModel/BulkCreateModels 三条写入路径；入站 associations 的 auto/disabledReason 以服务端现存状态为准。
4. 派生触发点：saveChannelCapabilities、SaveChannelEndpoints 后端点复核（失去端点能力支撑的声明按撤销处理，统计经 payload 返回）、auto_sync 模型增删联动、DeleteChannel/BulkDeleteChannels 清理、deriveModelAssociations（按模型名增量、只增不撤）。DuplicateChannel 不复制能力声明。
5. 运行时路由链路不变：仍按协议池 key 读取关联，auto 与手动条目运行期语义一致，禁止跨协议兜底。新增后端文件：`channel_capability.go`、`protocol_pool_derivation.go`、`model_capability_validation.go`。

## Model-centric Adapter Gateway（2026-08-02）

1. Adapter 只维护 `source_model_id -> model_id` 绑定；Channel、物理模型和协议能力属于 Model 的 `settings.protocolPools` 与 Channel endpoint。不要恢复 Channel 或物理模型字段到 Adapter binding。
2. Adapter 的入站 `apiFormat` 固定后，运行时只能读取对应协议池。协议池 key 当前为 `openai`（Chat Completions）、`openai_responses`（Responses）和 `anthropic`，完整请求格式由 `normalizeProtocolPoolKey`（概念名 `protocolPoolKey`）精确归一化后再查找；禁止跨协议兜底、隐式转换或回退到全渠道选择。
3. 协议池 key 与 Channel endpoint 的 `apiFormat` 不是同一字段：前者是 `openai`、`openai_responses`、`anthropic`，后者是 `openai/chat_completions`、`openai/responses`、`anthropic/messages` 等完整格式。前端用 `channelSupportsProtocolPool` 按协议映射过滤 endpoint；运行时仍须确认启用 Channel 存在匹配完整格式的 endpoint。

## Relay GID 边界

1. GraphQL Relay GID 在 Select 的 `value` 中保持原字符串；写入 REST 或 Ent 数字字段前，使用 `extractNumberID` 或 `extractNumberIDAsNumber` 转换。
2. 禁止裸写 `Number(gid)`，因为 GID 不是数字字符串，会得到 `NaN`。编辑回显时使用 `channelIdToSelectValue`、`modelIdToSelectValue` 等反向映射，让 Select 继续接收 GID。

## 变更入口

- Ent/GraphQL schema 和生成文件遵循 `.agent/rules/ent-graphql.md`；禁止手改 `internal/ent/` 和 gqlgen 生成文件。
- 配置保存后必须验证刷新后的 runtime snapshot；只看到数据库写入成功不能证明 Adapter 请求已使用新配置。
