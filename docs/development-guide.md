# Adapter Gateway 开发规范

用途：供后续 AI 与人类实现功能、审查改动时直接查阅，避免重复踩 GID、协议池、生成代码和协作隔离的坑。

## 1. Relay GID 与数字 ID

GraphQL Relay 返回的 ID 是类似下面的 GID，而不是可直接参与数值运算的字符串：

```text
gid://axonhub/Model/1
gid://axonhub/Channel/42
```

- 前端 Select 的 `value` **使用 GID 原值**，不要把它提前转成数字。
- 提交 REST/Ent 的数字字段前，必须用 `extractNumberID` 或 `extractNumberIDAsNumber` 转换。
- **禁止**裸写 `Number(gid)`；它会得到 `NaN`。
- 编辑回显时反向映射：Channel 使用 `channelIdToSelectValue`，Model 使用 `modelIdToSelectValue`，让 Select 继续拿到 GID。

```text
frontend/src/lib/utils.ts
frontend/src/features/models/data/protocol-pools.ts
frontend/src/features/adapters/index.tsx
```

## 2. 协议池 key 与 endpoint format

- 后端 `Model.Settings.ProtocolPools` 的 key 是协议族字符串：`openai` 或 `anthropic`。
- 前端协议池 UI 的 `format` 选项也必须使用协议族，不要写完整 endpoint 格式。
- Channel 的 `endpoints.apiFormat` 是完整格式，例如 `openai/chat_completions`；它不是协议池 key。
- 过滤 Channel 时使用 `channelSupportsProtocolPool` 做前缀匹配：协议池 `openai` 可匹配 `openai/...`，不能匹配 `anthropic/...`。
- Adapter selector 只查入站协议对应的 pool，**禁止跨协议兜底或隐式转换**。

## 3. Ent 与 GraphQL 生成代码

- **禁止手改 `internal/ent/` 下 generated 文件**；生成文件会在下一次 codegen 被覆盖。
- 修改 Ent schema 或 GraphQL schema 后，运行项目规定的生成入口，并把 schema 与生成结果一起审查：

```bash
make generate
# 等价入口：cd internal/server/gql && go generate
```

- 生成失败先修 schema/生成配置，不要直接编辑 generated 文件绕过错误。

## 4. 前端类型检查

类型检查命令如下：

```bash
cd frontend && pnpm exec tsc -p tsconfig.app.json --noEmit
```

当前基线约有 **175 条既有错误**。验证时记录基线，新增错误必须修复；不能把本次新增错误归入基线，也不要因基线错误而跳过定向检查。

## 5. Worktree 与子代理

- 每个功能使用独立 worktree；集成 worktree **只做 cherry-pick 合并**。
- 不并发修改同一文件；共享契约由一个 owner 维护，其他代理通过提交或明确接口协作。
- 前端修改使用 `frontend-worker`；前端审查使用 `frontend-reviewer`；后端或全栈任务使用 `claude-code`。
- 子代理开始前声明允许/禁止修改的文件，完成后给出变更文件和验证证据。

## 6. 最小改动原则

先读调用方、schema、resolver、测试和相关文档，再按现有模式做最小完整改动。Adapter binding 只写 `source_model_id → model_id`；Channel、物理模型和协议能力属于 Model 的协议池/Channel endpoint。