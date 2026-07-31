# 模型组能力元数据实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `subagent-driven-development` 或 `executing-plans` 按任务逐项执行。每个任务都应独立验证并提交。

**目标：** 在已跑通的 Adapter/ModelGroup MVP 上，逐步补齐物理模型的上下文限制、输出 Token、工具调用、多模态、推理能力、价格和 `/models` 元数据。

**架构：** 能力与价格首先归属于 ModelGroupTarget，因为同一个逻辑模型组可以绑定不同渠道和不同物理模型。ModelGroup 只负责对外逻辑模型与路由策略；对外展示的聚合能力在后续根据启用目标计算。已有旧 `Model.model_card` 和 `ChannelModelPrice` 作为兼容数据源，不复制价格数据。

**技术栈：** Go、Ent、Gin、React、TypeScript、TanStack Query、现有 JSON 字段迁移机制。

---

## 范围与兼容约定

1. `ModelGroupTarget.capabilities` 已经是 JSON 字段，本计划第一阶段扩展 JSON 结构，不新增数据库列，避免破坏现有数据库。
2. 新增数字字段使用 `0` 表示“未声明”，不能把未知能力误判为不支持。
3. `supports_tools=false`、`supports_stream=false` 等已有字段继续保持当前语义；缺失的新字段按未知处理。
4. 价格继续读取 `ChannelModelPrice` / `objects.ModelPrice`，ModelGroupTarget 只保存可选的价格引用或使用 `(channel_id, target_model_id)` 查找，不复制完整价格结构。
5. 物理模型能力自动继承先作为独立阶段，第一阶段允许在 ModelGroup 页面手工配置，保证每个阶段都能独立上线和验证。

---

## 阶段一：补齐目标级能力字段与配置表单

**结果：** 每个 ModelGroupTarget 可以保存并编辑上下文窗口、最大输出 Token、推理能力和完整输入输出模态；旧目标数据继续可读写。

**文件：**
- 修改：`internal/objects/adapter.go`
- 修改：`internal/server/biz/adapter.go`
- 修改：`internal/server/api/gateway.go`
- 修改：`frontend/src/lib/adapterApi.ts`
- 修改：`frontend/src/features/model-groups/index.tsx`
- 测试：`internal/server/api/gateway_test.go`
- 测试：`internal/server/orchestrator/adapter_selector_test.go`

### 步骤 1：先写后端能力结构测试

在 `internal/server/api/gateway_test.go` 增加一个更新目标能力的测试，发送如下请求体：

```json
{
  "display_name": "GPT 5",
  "status": "enabled",
  "selection_strategy": "priority_failover",
  "protocols": [
    {
      "inbound_api_format": "openai/chat_completions",
      "enabled": true,
      "targets": [
        {
          "channel_id": 1,
          "target_model_id": "gpt-5",
          "outbound_api_format": "openai/chat_completions",
          "priority": 1,
          "enabled": true,
          "capabilities": {
            "supports_tools": true,
            "supports_stream": true,
            "stream_policy": "unlimited",
            "supports_reasoning": true,
            "context_length": 200000,
            "max_output_tokens": 8192,
            "input_modalities": ["text", "image"],
            "output_modalities": ["text"]
          }
        }
      ]
    }
  ]
}
```

断言更新接口成功，并再次调用列表接口，确认返回值保留：

```text
supports_reasoning=true
context_length=200000
max_output_tokens=8192
input_modalities=[text,image]
```

运行：

```bash
go test ./internal/server/api -run 'Test.*ModelGroup' -count=1
```

预期：新增断言在当前实现下失败，证明字段尚未贯通。

### 步骤 2：扩展领域对象和 DTO

在 `internal/objects/adapter.go` 的 `AdapterTargetCapabilities` 增加：

```go
SupportsReasoning  bool     `json:"supports_reasoning"`
ContextLength      int      `json:"context_length,omitempty"`
MaxOutputTokens    int      `json:"max_output_tokens,omitempty"`
```

同步修改：

```text
internal/server/biz/adapter.go
  AdapterTargetCapabilitiesInput
  cloneTargetCapabilities
  UpdateModelGroup 的目标转换

internal/server/api/gateway.go
  TargetCapabilitiesDTO
  TargetInput
  convertProtocolInputs
  ListModelGroups 的 DTO 转换
```

对 `ContextLength` 和 `MaxOutputTokens` 增加非负校验；允许 `0` 表示未声明。`supports_reasoning` 直接作为布尔值保存。

### 步骤 3：运行后端测试并修复旧数据兼容

运行：

```bash
go test ./internal/server/api ./internal/server/biz ./internal/server/orchestrator -count=1
```

必须满足：

- 旧 JSON 中没有新增字段时仍能反序列化；
- 旧目标可以被列表接口返回；
- 旧目标更新时不因为缺少新字段而失败；
- 新能力字段完成持久化和读取。

### 步骤 4：扩展前端类型和默认值

在 `frontend/src/lib/adapterApi.ts` 的 `TargetCapabilities` 增加：

```ts
supports_reasoning: boolean;
context_length?: number;
max_output_tokens?: number;
```

在 `EMPTY_CAPABILITIES` 中新增：

```ts
supports_reasoning: false,
context_length: 0,
max_output_tokens: 0,
```

`0` 在界面显示为“未限制/未声明”，提交时保持为 `0`。

### 步骤 5：增加目标能力编辑控件

在 `frontend/src/features/model-groups/index.tsx` 的目标新增和目标编辑区域增加：

- 上下文窗口：数字输入，单位 Token；
- 最大输出 Token：数字输入；
- 支持工具调用：开关；
- 支持推理：开关；
- 输入模态：`text`、`image`、`video`、`audio` 多选；
- 输出模态：`text`、`image`、`video`、`audio` 多选；
- 现有流式策略继续保留。

新增目标和编辑已有目标必须共用同一组字段转换函数，避免新增时保存、编辑时丢失。

### 步骤 6：运行前端静态检查并提交

运行：

```bash
cd frontend
pnpm lint
pnpm build
cd ..
git diff --check
```

提交：

```bash
git add internal/objects/adapter.go internal/server/biz/adapter.go internal/server/api/gateway.go internal/server/api/gateway_test.go internal/server/orchestrator/adapter_selector_test.go frontend/src/lib/adapterApi.ts frontend/src/features/model-groups/index.tsx
git commit -m "feat: 补充模型组目标能力配置"
```

---

## 阶段二：把能力用于目标筛选

**结果：** 路由选择不再只校验流式、工具和模态，还能拒绝超过目标限制的请求。

**文件：**
- 修改：`internal/server/orchestrator/adapter_selector.go`
- 修改：`internal/server/orchestrator/candidates.go`
- 修改：`internal/server/orchestrator/select_candidates.go`
- 测试：`internal/server/orchestrator/adapter_selector_test.go`
- 测试：`internal/server/orchestrator/select_candidates_test.go`

### 步骤 1：先写筛选测试

覆盖以下情况：

```text
请求 max_tokens <= max_output_tokens：目标可选
请求 max_tokens > max_output_tokens：目标被排除
目标 max_output_tokens=0：不因未知限制被排除
请求包含 image，目标 input_modalities 不含 image：目标被排除
请求包含 tools，目标 supports_tools=false：目标被排除
```

### 步骤 2：实现目标能力条件

在现有 `candidate` 条件链中增加最大输出 Token 校验。上下文窗口校验只在已有请求 Token 统计可用时执行；没有可靠 Token 统计时不估算，不把请求错误地拒绝。

### 步骤 3：验证和提交

```bash
go test ./internal/server/orchestrator -count=1
 git add internal/server/orchestrator
git commit -m "feat: 按模型能力筛选网关目标"
```

---

## 阶段三：让 Adapter `/models` 返回逻辑模型元数据

**结果：** 客户端调用 `/{adapter}/v1/models` 时可以看到模型能力，而不只是模型 ID。

**文件：**
- 修改：`internal/objects/adapter.go`
- 修改：`internal/server/biz/adapter.go`
- 修改：`internal/server/api/adapter.go`
- 测试：`internal/server/api/adapter_test.go`

### 聚合规则

对于同一个逻辑模型组的多个启用目标：

- `context_length`：取所有目标的最小非零值；没有可用值则省略；
- `max_output_tokens`：取所有目标的最小非零值；没有可用值则省略；
- 输入/输出模态：取所有目标的交集，保证声明的能力对所有故障转移目标都成立；
- `supports_tools`、`supports_reasoning`：所有目标都支持才返回 `true`；
- 价格：阶段五之前不在 `/models` 聚合响应中虚构单一价格。

### 实现步骤

1. 为逻辑模型定义只读元数据结构；
2. 在 AdapterService 根据当前快照聚合目标能力；
3. 扩展 `adapterOpenAIModel` 和 `adapterAnthropicModel`；
4. 测试多目标交集、未知值和单目标场景；
5. 提交：

```bash
git add internal/objects/adapter.go internal/server/biz/adapter.go internal/server/api/adapter.go internal/server/api/adapter_test.go
git commit -m "feat: 返回网关逻辑模型能力元数据"
```

---

## 阶段四：兼容旧 ModelCard，自动填充物理模型能力

**结果：** 新增 ModelGroupTarget 时，如果旧模型目录中已有能力资料，页面可以自动带出，用户仍可以覆盖。

**文件：**
- 修改：`internal/server/biz/adapter.go`
- 修改：`internal/server/api/gateway.go`
- 修改：`frontend/src/features/model-groups/index.tsx`
- 测试：`internal/server/biz/adapter_test.go`
- 测试：`internal/server/api/gateway_test.go`

### 数据匹配规则

1. 优先按 `target_model_id` 查询旧 `Model.model_id`；
2. 查不到时使用手工能力默认值；
3. 旧 `ModelCard` 的 `Limit.Context` 映射到 `context_length`；
4. 旧 `ModelCard` 的 `Limit.Output` 映射到 `max_output_tokens`；
5. `Modalities` 映射到输入输出模态；
6. `Vision` 映射为输入模态包含 `image`；
7. `ToolCall` 映射到 `supports_tools`；
8. `Reasoning.Supported` 映射到 `supports_reasoning`；
9. 手工已经保存的目标能力不被自动刷新覆盖。

由于旧 `Model` 没有和 `channel_id` 建立外键，本阶段不得假设同名模型在所有渠道上能力一致；查询结果只能作为默认值，页面必须显示“来自模型目录”或“手工配置”的来源。

---

## 阶段五：接入渠道模型价格

**结果：** ModelGroupTarget 能显示对应渠道物理模型的价格，并且不复制现有价格结构。

**文件：**
- 修改：`internal/server/biz/adapter.go`
- 修改：`internal/server/api/gateway.go`
- 修改：`frontend/src/lib/adapterApi.ts`
- 修改：`frontend/src/features/model-groups/index.tsx`
- 复用：`internal/ent/schema/channel_model_price.go`
- 复用：`internal/objects/price.go`
- 测试：`internal/server/api/gateway_test.go`

### 实现规则

1. 使用 `(channel_id, target_model_id)` 查询 `ChannelModelPrice`；
2. 价格不写入 `ModelGroupTarget.capabilities`；
3. API 返回价格的完整结构和 `reference_id`；
4. 没有价格配置时返回空值，不伪造价格；
5. 价格编辑继续复用渠道页面已有的价格编辑器；
6. ModelGroup 页面先提供查看入口，不在第一版复制完整价格编辑器。

---

## 阶段六：运行验收与文档

**文件：**
- 修改：`docs/zh/development/development.md`
- 修改：`docs/zh/deployment/docker.md`
- 修改：`README.zh-CN.md`
- 测试：`internal/server/api/*_test.go`
- 测试：`internal/server/orchestrator/*_test.go`

### 验收场景

1. 创建单目标模型组，填写上下文窗口、最大输出 Token、工具和图片能力；
2. 刷新页面，能力配置仍存在；
3. 绑定到 Adapter，通过 `/{adapter}/v1/models` 查看能力；
4. 请求超过最大输出 Token，路由返回明确错误；
5. 图片请求只命中声明支持图片的目标；
6. 两个目标能力不同，模型组对外返回交集能力；
7. 删除价格配置后，页面和 `/models` 不显示虚假价格；
8. 旧数据库目标没有新增 JSON 字段时，服务正常启动和查询。

### 最终验证命令

```bash
go test ./internal/server/api ./internal/server/biz ./internal/server/orchestrator -count=1
cd frontend && pnpm lint && pnpm build
cd ..
git diff --check
docker compose up -d --build --force-recreate
docker compose ps
curl http://localhost:8090/health
```

---

## 执行顺序

按以下顺序逐项完成，每阶段单独提交并验证：

```text
阶段一：字段 + API + UI
阶段二：路由筛选
阶段三：/models 元数据
阶段四：旧 ModelCard 自动填充
阶段五：价格
阶段六：验收与文档
```

第一步只做阶段一，不在同一提交中混入价格、自动继承或路由聚合。
