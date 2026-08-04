# 前端技术架构与开发规范

本文档面向 AxonHub 前端开发与验收，内容以当前 `frontend/src` 实现为准。

## 1. 技术栈与分层

- **运行时与构建**：React 19、TypeScript、Vite。入口为 [`frontend/src/main.tsx`](../../frontend/src/main.tsx)，依赖和脚本见 [`frontend/package.json`](../../frontend/package.json)。使用 `pnpm` 管理依赖和执行命令。
- **路由**：TanStack Router 文件路由。根路由见 [`frontend/src/routes/__root.tsx`](../../frontend/src/routes/__root.tsx)，登录/错误页面位于 `routes/(auth)`、`routes/(errors)`，需登录的页面位于 [`frontend/src/routes/_authenticated`](../../frontend/src/routes/_authenticated)。`_authenticated/route.tsx` 统一接入 `AuthGuard` 和 `AuthenticatedLayout`，页面入口再通过 `RouteGuard` 做 scope 校验。
- **服务端状态**：TanStack Query。GraphQL 请求封装在 [`frontend/src/gql/graphql.ts`](../../frontend/src/gql/graphql.ts)，各领域查询/变更通常在 feature 的 `data` 目录或 `gql` 目录中定义；成功变更后应按 query key 失效缓存。
- **客户端状态**：Zustand 用于跨页面/全局状态（例如认证、主题等）；局部表单、弹窗和编辑草稿使用 React `useState`/`useEffect`，不要把短生命周期表单状态提升为全局状态。
- **UI**：Tailwind CSS v4；`frontend/src/components/ui` 基于 Radix UI 原语封装，采用 shadcn/ui 风格的组合组件和 `data-slot` 标记。优先复用现有 `Dialog`、`Select`、`Button`、表格等组件，不直接在业务页面复制原语封装。

## 2. 页面、路由与 feature 的关系

TanStack Router 的文件路径只负责 URL、鉴权和页面装配，业务实现放在 `features`：

```text
frontend/src/routes/_authenticated/<page>/index.tsx
  └─ RouteGuard / 页面容器
      └─ frontend/src/features/<feature>/index.tsx
          ├─ components/   页面组件、表格、弹窗
          ├─ data/         GraphQL 查询、schema、表单数据模型
          ├─ hooks/        feature 专用 hooks
          ├─ context/      feature 局部上下文
          └─ utils/        格式化、合并和校验工具
```

- 路由入口示例：[`routes/_authenticated/channels/index.tsx`](../../frontend/src/routes/_authenticated/channels/index.tsx)、[`routes/_authenticated/adapters/index.tsx`](../../frontend/src/routes/_authenticated/adapters/index.tsx)、[`routes/_authenticated/models/index.tsx`](../../frontend/src/routes/_authenticated/models/index.tsx)。
- **models** 是逻辑 Model 管理页面，入口实现见 [`features/models/index.tsx`](../../frontend/src/features/models/index.tsx)。Model 编辑器维护 `settings.protocolPools`，以协议族组织 Channel/物理模型关联。
- **adapters** 是网关入站协议适配器管理页面，入口实现见 [`features/adapters/index.tsx`](../../frontend/src/features/adapters/index.tsx)。Adapter 只绑定 source model 与 Model，属于网关运行时配置。
- **channels** 是真实下游渠道管理域。Model 协议池编辑器通过 Channel GraphQL 查询提供渠道选择和 endpoint 过滤；修改某一域时保持域边界，不在路由组件中直接拼接跨域请求。

## 3. 数据访问边界

### 3.1 GraphQL：Model 与 Channel 管理

Model 和 Channel 都使用管理 GraphQL 端点 `/admin/graphql`，请求封装在 [`frontend/src/gql/graphql.ts`](../../frontend/src/gql/graphql.ts)。Model 查询在 [`features/models/data/models.ts`](../../frontend/src/features/models/data/models.ts) 中集中定义，并显式返回 `settings.protocolPools`；Model 新建/编辑通过 GraphQL mutation 提交完整 settings。

Model 编辑器读取可用渠道：

```ts
useQueryChannels({ first: 2000 });
```

因此：

- Model 的 `settings.protocolPools[].format` 使用协议族（当前 UI 为 `openai`、`anthropic`）；
- `channelModel.channelId` 和 GraphQL Channel 的 `id` 在 UI 边界需要按 [协议池规则](../../.agent/rules/adapter-model-binding.md) 转换；
- GraphQL 查询/变更使用现有 feature 封装，不要在组件里自行 `fetch('/admin/graphql')`；
- 成功更新 Model 后应失效对应 Model 查询缓存，确保列表和编辑回显读取新 settings。

### 3.2 REST：Adapter 运行时配置

[`frontend/src/lib/adapterApi.ts`](../../frontend/src/lib/adapterApi.ts) 专门承载 Adapter 和 runtime 管理 API：

| Hook/函数 | 请求 | 用途 |
| --- | --- | --- |
| `useAdapters` | `GET /admin/gateway/adapters` | 读取 Adapter 与 binding |
| `useUpsertAdapter` | `PUT /admin/gateway/adapters/:name` | 整体保存 `source_model_id -> model_id` binding |
| `useRefreshGateway` | `POST /admin/gateway/refresh` | 发布新的运行时 snapshot |
| `useGatewayRuntime` | `GET /admin/gateway/runtime` | 查看运行时 snapshot 与诊断 |
| `useRenameAdapter` / `useDeleteAdapter` | `POST .../rename` / `DELETE ...` | Adapter 生命周期操作 |

`AdapterUpdateInput` 是 REST 请求契约：binding 的 `model_id` 必须是 number，不能把 Relay GID 原样写入 payload。Adapter 保存成功不等于请求已经使用新配置；需要刷新并重新读取 runtime。

## 4. Model 协议池编辑状态流（2026-08-02）

实现集中在 [`frontend/src/features/models/components/models-action-dialog.tsx`](../../frontend/src/features/models/components/models-action-dialog.tsx)：

1. 打开新建/编辑弹窗时，表单初始化 `settings.protocolPools`；编辑已有 Model 时从 `currentRow.settings.protocolPools` 回填。
2. 协议池 `format` 使用协议族 key，当前可选 `openai`、`anthropic`；不要写 `openai/chat_completions` 等完整 endpoint 格式。
3. 每个协议池的 association 使用 `type: 'channel_model'`，保存 Channel numeric ID、物理模型 ID、priority 和 `disabled`。Channel 下拉选项通过 `channelSupportsProtocolPool` 按 endpoint 前缀过滤。
4. `Select` 的 Channel value 保持 GraphQL Relay GID；写入 association 前通过 `parseChannelIdFromSelectValue` 提取数字，编辑回显通过 `channelIdToSelectValue` 映射回 GID。
5. 保存前必须校验协议池格式非空、每个池至少有一个 target、priority 为 0 到 100 的整数，并拒绝重复的 Channel/物理模型组合。
6. 更新 Model 时以 GraphQL mutation 提交完整 `settings.protocolPools`；Adapter 绑定在独立的 Adapter REST 页面维护，不把 Channel 或物理模型写回 Adapter binding。

## 4.1 渠道能力编辑器与派生条目（2026-08-05）

- **渠道能力编辑器**：[`channels-capability-dialog.tsx`](../../frontend/src/features/channels/components/channels-capability-dialog.tsx)，行菜单入口（`channels-columns.tsx`，对话框类型 `'capability'` 在 `channels-context.tsx`，挂载在 `channels-dialogs.tsx`）。交互：协议族多选（白名单复用 models 域 `protocolPoolFormats`，跨 feature import）→ 模型候选默认继承全部协议 → 逐模型去掉例外（持久保存）。
- **保存与提示**：`useSaveChannelCapabilities` 提交 `saveChannelCapabilities`，payload 返回 `addedCount`/`unmatchedModels`/`revoked`，保存后提示新增关联数与未匹配声明，并提供一键批量启用（`useBulkEnableDerivedAssociations`，逐条校验、部分成功）；`saveChannelEndpoints` 返回改为 payload，端点复核产生的撤销同样提示。
- **缓存失效**：能力保存/批量启用写入的是各 Model settings，onSuccess 需同时 invalidate `['channels']`、`['channel', id]` 与 `['models']`；`useDeriveModelAssociations` 后 invalidate `['models']` 及模型详情 key。
- **派生条目展示**：Model 协议池编辑器中 `auto: true` 关联展示 auto 徽章与 `disabledReason`，隐藏删除按钮（服务端同样拒绝删除）；"同步渠道"按钮调 `deriveModelAssociations`（增量、只增不撤）；启用校验失败时服务端拒绝保存并展示原因。

## 5. 已验证的关键坑

- **ID 不同**：GraphQL 返回 global ID，REST/Ent 数值字段需要 `extractNumberID` 或 `extractNumberIDAsNumber`；不要直接对 GID 使用 `Number()`。
- **协议池 key 不等于 endpoint format**：协议池按协议族隔离，Channel endpoint 使用完整 `apiFormat`；UI 过滤和 runtime 选择都不能跨协议兜底。
- **Model 是共享中心**：多个 Adapter 可以绑定同一个 Model；修改 Model 的协议池并刷新 snapshot 后，引用它的 Adapter 共同使用新配置。
- **保存不是运行时刷新**：Model/Adapter 持久化成功后仍需刷新 runtime；验收时同时检查 GraphQL/REST 响应和 `/admin/gateway/runtime`。
- **弹窗内容需要滚动**：Dialog 内的 Select/Popover 应使用现有 `portalContainer` 约定，宽表格优先保证容器可滚动，不让 intrinsic width 撑破视口。

## 6. 开发与验证规范

### 开发

- 使用 `pnpm`：在 `frontend/` 目录执行依赖安装和前端验证命令；不要混用 npm/yarn 生成 lockfile。
- 修改 Model 或 Adapter 字段时同步检查 GraphQL schema/查询、REST 类型与 hook、表单映射、query key 和 i18n 文案。
- 跨 feature 读取数据应调用已有 hook，不复制请求逻辑；涉及 GID、协议池和 Adapter binding 时先确认数据来源和字段类型。
- 联调命令、Network、Console 和日志中不得写入或打印 credentials、JWT、API Key；示例使用占位符并脱敏响应。

### 验收

浏览器和定向测试命令维护在 [`.agent/rules/e2e.md`](../../.agent/rules/e2e.md)。页面或弹窗改动至少检查：

1. snapshot 确认路由、权限守卫、弹窗层级、表单字段和空/加载/错误状态；
2. Network 确认 GraphQL Model/Channel 查询、REST Adapter 请求的方法和 payload，特别是 `model_id`/`channelId` 的 numeric 边界；
3. Console 无 React、GraphQL、请求失败或受控组件警告；不记录 token 或 credentials；
4. 保存后重新打开页面，确认协议池、Channel/物理模型关联、Adapter binding 和刷新后的 runtime 结果一致。
