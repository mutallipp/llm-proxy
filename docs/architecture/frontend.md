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

- 路由入口示例：[`routes/_authenticated/channels/index.tsx`](../../frontend/src/routes/_authenticated/channels/index.tsx)、[`routes/_authenticated/adapters/index.tsx`](../../frontend/src/routes/_authenticated/adapters/index.tsx)、[`routes/_authenticated/model-groups/index.tsx`](../../frontend/src/routes/_authenticated/model-groups/index.tsx)。
- 渠道管理实现见 [`features/channels`](../../frontend/src/features/channels)，查询和变更集中在 [`features/channels/data/channels.ts`](../../frontend/src/features/channels/data/channels.ts)，复杂表格操作放在 `components/`。
- **adapters** 是网关入站协议适配器管理页面，入口实现见 [`features/adapters/index.tsx`](../../frontend/src/features/adapters/index.tsx)。Adapter 绑定 source model 与 model group，属于网关运行时配置，不应与渠道 GraphQL CRUD 混写。
- **model-groups** 是模型组编辑页面，入口实现见 [`features/model-groups/index.tsx`](../../frontend/src/features/model-groups/index.tsx)。它以入站协议（protocol）组织多个出站目标（target），目标引用渠道和目标模型。
- **channels** 是真实下游渠道的管理域。model-groups 编辑器通过 channels 查询提供渠道选择、可用出站格式和测试入口；保存模型组仍走网关 REST API。修改某一域时保持域边界，不在路由组件中直接拼接跨域请求。

## 3. 数据访问边界

### 3.1 GraphQL：渠道业务查询

渠道列表使用管理 GraphQL 端点 `/admin/graphql`，请求封装在 [`frontend/src/gql/graphql.ts`](../../frontend/src/gql/graphql.ts)。[`useQueryChannels`](../../frontend/src/features/channels/data/channels.ts) 执行 `QUERY_CHANNELS_QUERY`，返回 connection 后再通过 schema 解析；筛选、分页、轮询和 `['channels', ...]` query key 由该 hook 负责。

模型组编辑器只读取可用渠道：

```ts
useQueryChannels({
  first: 100,
  where: { statusIn: ['enabled', 'disabled'] },
});
```

因此：

- 渠道名称、GraphQL channel 对象、渠道端点/格式等来源于 GraphQL；
- GraphQL channel 的 `id` 是 global ID（通常包含路径段），展示或传给 REST 数值字段前必须转换；
- GraphQL 查询/变更使用现有 `gql`/`data` 封装和 schema，不要在组件里自行 `fetch('/admin/graphql')`。

### 3.2 REST：网关运行时配置

[`frontend/src/lib/adapterApi.ts`](../../frontend/src/lib/adapterApi.ts) 专门承载 adapters/model-groups 的 REST 管理 API：

| Hook/函数 | 请求 | 用途 |
| --- | --- | --- |
| `useAdapters` | `GET /admin/gateway/adapters` | 读取 Adapter 配置 |
| `useModelGroups` | `GET /admin/gateway/model-groups` | 读取模型组配置 |
| `useUpsertAdapter` | `PUT /admin/gateway/adapters/:name` | 整体保存 Adapter |
| `useUpsertModelGroup` | `PUT /admin/gateway/model-groups/:name` | 整体保存模型组 |
| `useRefreshGateway` | `POST /admin/gateway/refresh` | 刷新网关运行时快照 |
| `useGatewayRuntime` | `GET /admin/gateway/runtime` | 查看运行时快照 |
| `useRenameAdapter` / 删除 hooks | `POST .../rename` / `DELETE ...` | Adapter、模型组生命周期操作 |

`adapterApi.ts` 中的 TypeScript 类型是请求和响应契约：`ModelGroupUpdateInput` 包含 `display_name`、`status`、`selection_strategy`、`remark` 和完整 `protocols`；`ModelGroupTarget` 的 `channel_id` 必须是 number。新增或修改 API 字段时，必须同步类型、调用 hook、表单映射和成功后的缓存失效逻辑。

## 4. ModelGroup 编辑弹窗状态流

实现集中在 [`frontend/src/features/model-groups/index.tsx`](../../frontend/src/features/model-groups/index.tsx)，编辑器按以下规则工作：

1. 打开新建/编辑弹窗时，用 `EMPTY_GROUP` 或已有模型组初始化 `name` 与 `draft`；`draft` 的类型是 `ModelGroupUpdateInput`。`useEffect` 在弹窗打开时重置协议格式和各协议的目标草稿，避免沿用上一次编辑内容。
2. **协议草稿**在 `draft.protocols` 中，字段包括 `inbound_api_format`、`enabled`、`remark`、`targets`。协议格式来自 `INBOUND_API_FORMATS`（OpenAI Chat Completions、OpenAI Responses、Anthropic Messages），同一模型组内不能重复添加。
3. **目标草稿**放在 `newTargets[protocolIndex]`，只表示“新增目标表单”当前输入：`channel_id`、`target_model_id`、`outbound_api_format`、`enabled`、`stream_policy`。它不是已保存目标，也不会自动进入 `draft.protocols[].targets`。
4. 点击“添加”并通过渠道和目标模型校验后，才将目标复制进对应协议的 `targets`；同时按当前列表长度分配 `priority`，清空草稿（保留渠道选择）。未点击“添加”时，目标仍只是草稿，直接保存不会提交该目标，且可能因缺少目标配置而失败。
5. 保存是**整体替换**：`useUpsertModelGroup().mutateAsync({ name, data: draft })` 将当前完整协议和目标集合 PUT 到 `/admin/gateway/model-groups/:name`。删除目标、调整优先级等操作必须先更新 `draft`，不能只改 UI 列表。
6. 目标的 `capabilities` 默认至少包含 `supports_tools: true`、`supports_stream: true`、`input_modalities: ['text']`、`output_modalities: ['text']`，`stream_policy` 默认 `unlimited`。`stream_policy` 只允许三种值：`unlimited`（跟随下游）、`require`（强制流式）、`forbid`（禁止流式）。编辑既有目标时保留其他 capabilities，只覆盖该字段。
7. `channel_id` 在 UI 选择时可暂存为字符串，但写入 `ModelGroupTargetInput` 前必须转成 number。保存成功后 REST hook 会失效 `gateway-model-groups`；若需要立即验证网关行为，点击“刷新快照”调用 `POST /admin/gateway/refresh`，随后重新读取 runtime snapshot（`useGatewayRuntime`）。

## 5. 已验证的关键坑

- **ID 不同**：GraphQL 返回 global ID，REST 的 `channel_id` 要 numeric ID。统一使用 [`extractNumberID`](../../frontend/src/lib/utils.ts) 提取最后一个路径段；不要直接对 global ID 使用 `Number()`，也不要把 global ID 原样写入 REST payload。
- **目标必须先添加**：只填写目标草稿不等于加入协议目标池。保存前检查目标是否已经出现在 `draft.protocols[].targets`，否则会漏提交或触发后端校验失败。
- **弹窗宽度不是内容宽度**：通用 Dialog 默认 `sm:max-w-lg`。模型组编辑器使用响应式宽度（`w-[96vw]`、`max-h-[90vh]`、纵向滚动）；新增目标的宽表格使用 `min-w-[1100px]` 和横向 `overflow-x-auto`。Dialog、grid、Select 的 intrinsic width 不应撑破视口，移动端先保证容器可滚动，再逐列收缩布局。
- **保存不是运行时刷新**：保存模型组只更新配置并返回 `snapshot_version`/`refreshed_at` 等信息；验收实际路由行为前必须刷新并重新读取 runtime snapshot，不能只看 toast 或列表缓存。

## 6. 开发与验证规范

### 开发

- 使用 `pnpm`：在 `frontend/` 目录执行依赖安装、开发和验证命令；不要混用 npm/yarn 生成 lockfile。
- 改组件时同步检查 TypeScript 类型、GraphQL schema/查询、REST 类型与 hook、query key 和 i18n 文案。跨 feature 读取数据应调用已有 hook，不复制请求逻辑。
- 涉及渠道 ID、模型组协议、适配器绑定时，先确认数据来源（GraphQL 或 REST）和字段类型，再做转换；不要用隐式字符串/数字转换掩盖契约差异。
- 联调命令、curl、网络面板和日志中不得写入或打印 credentials、JWT、API Key；示例使用占位符并脱敏响应。

### 验收

对页面或弹窗改动，至少按以下顺序检查：

1. 用浏览器 snapshot 确认路由、权限守卫、弹窗层级、表单字段和空/加载/错误状态；重点检查 Dialog、grid、Select 在窄视口下没有溢出。
2. 用 Network 检查 GraphQL `channels` 查询的变量与分页/筛选，以及 REST `GET/PUT /admin/gateway/*` 的路径、HTTP 方法和 payload；确认 `channel_id` 为 numeric ID。
3. 用 Console 检查是否有 React、GraphQL、请求失败或受控组件警告；不要把 token 或 credentials 粘贴到控制台记录。
4. 对保存流程截图留存关键状态：目标草稿、点击“添加”后的目标池、保存成功、刷新 snapshot 后的运行时结果。验证目标协议、三种 `stream_policy` 和 `supports_tools` 默认值。
5. 本任务范围内只运行与改动匹配的验证命令；文档或代码检查前先确认不会启动 dev server。