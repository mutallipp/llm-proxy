---
alwaysApply: false
globs: "frontend/src/features/adapters/**/*.ts, frontend/src/features/models/**/*.ts, frontend/src/lib/adapterApi.ts, internal/objects/model.go, internal/objects/adapter.go, internal/server/orchestrator/**/*.go, internal/server/biz/adapter*.go, internal/server/api/adapter*.go, internal/server/api/gateway.go"
---

# Adapter / Model 绑定规则

## Model-centric Adapter Gateway（2026-08-02）

1. Adapter 只维护 `source_model_id -> model_id` 绑定；Channel、物理模型和协议能力属于 Model 的 `settings.protocolPools` 与 Channel endpoint。不要恢复 Channel 或物理模型字段到 Adapter binding。
2. Adapter 的入站 `apiFormat` 固定后，运行时只能读取对应协议池。协议池 key 是协议族（当前为 `openai`、`anthropic`），完整请求格式由 `normalizeProtocolPoolKey`（概念名 `protocolPoolKey`）归一化后再查找；禁止跨协议兜底、隐式转换或回退到全渠道选择。
3. 协议池 key 与 Channel endpoint 的 `apiFormat` 不是同一字段：前者是 `openai`/`anthropic`，后者是 `openai/chat_completions`、`anthropic/messages` 等完整格式。前端用 `channelSupportsProtocolPool` 按协议前缀过滤 endpoint；运行时仍须确认启用 Channel 存在匹配完整格式的 endpoint。

## Relay GID 边界

1. GraphQL Relay GID 在 Select 的 `value` 中保持原字符串；写入 REST 或 Ent 数字字段前，使用 `extractNumberID` 或 `extractNumberIDAsNumber` 转换。
2. 禁止裸写 `Number(gid)`，因为 GID 不是数字字符串，会得到 `NaN`。编辑回显时使用 `channelIdToSelectValue`、`modelIdToSelectValue` 等反向映射，让 Select 继续接收 GID。

## 变更入口

- Ent/GraphQL schema 和生成文件遵循 `.agent/rules/ent-graphql.md`；禁止手改 `internal/ent/` 和 gqlgen 生成文件。
- 配置保存后必须验证刷新后的 runtime snapshot；只看到数据库写入成功不能证明 Adapter 请求已使用新配置。
