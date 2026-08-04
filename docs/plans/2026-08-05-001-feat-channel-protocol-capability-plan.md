---
title: 渠道协议能力声明与协议池自动派生 - Plan
type: feat
date: 2026-08-05
topic: channel-protocol-capability
artifact_contract: ce-unified-plan/v1
artifact_readiness: implementation-ready
product_contract_source: ce-brainstorm
deepened: true
---

# 渠道协议能力声明与协议池自动派生 - Plan

## Goal Capsule

- **Objective:** 让渠道声明"每个模型支持哪些协议"，系统据此为同名逻辑模型自动派生（默认关闭的）协议池关联；接新渠道只需启用，不再逐个模型手配。
- **Product authority:** 本计划只覆盖协议能力声明与派生这一个工作单元。Adapter 绑定结构、新协议族接入、端点配置重构、事件驱动刷新机制均为周边工作，不在活跃范围。
- **Open blockers:** 无；技术取舍已由 ce-plan 拍板（KTD14-KTD20），计划已达 implementation-ready。

## Product Contract

### Summary

把"模型走哪个协议"的事实来源从 Model 侧人工配置改为渠道侧声明：渠道先多选支持的协议族（须与端点实际能力一致），添加的模型默认继承全部协议、可逐个去掉；保存后系统按同名精确匹配为逻辑模型派生协议池关联（默认禁用、带自动标记）。派生条目一经用户启用即转为手动条目，脱离自动对账处置；任何启用操作都须通过渠道能力校验。Model 侧只做启停、优先级和例外手动配置；存量显式关联零迁移继续生效。

### Problem Frame

当前协议池关联完全靠 Model 侧逐条手配渠道+物理模型，存在两个问题：接新渠道时要为每个模型挨个添加关联；配置者若不了解渠道的真实协议能力（如 kiro 的 opus-4.7 只支持 anthropic、gpt-5.6-luna 只支持 openai），容易误配——而运行时端点选择会静默跨协议转换（`SelectAPIFormat` 按请求类型能力列表兜底，最终兜底第一个端点），误配不报错、只在行为上劣化。协议支持本是上游供应商决定的渠道层客观事实，应归渠道声明，并在声明与启用时以渠道端点能力和运行时模型条目为准做校验。

### Actors

- A1. **平台运维**：配置渠道协议能力、启用/调整派生关联、为自定义名称模型做例外配置。
- A2. **API 调用方**：无感知；入站协议路由行为不变。

### Key Decisions

- KTD1. **协议×模型能力声明归渠道层**，Model 侧显式关联降级为例外覆盖。(session-settled: user-directed — chosen over Model 侧继续逐条手配：能力是上游供应商决定的客观事实，手配易误配且接新渠道负担重)
- KTD2. **派生结果物化写入各 Model 的 protocolPools**，不在请求路径计算。(session-settled: user-approved — chosen over 运行时计算：运行时路径不变、"默认关闭"状态可持久化；代价是渠道保存时一次批量派生写入)
- KTD3. **同名精确匹配自动关联，默认关闭**。(session-settled: user-directed — chosen over 默认启用：避免新接入渠道未经确认承接生产流量)
- KTD4. **优先级、条件路由保留为 Model 侧覆盖项**，渠道层只管"能不能"，Model 层管"怎么选"。(session-settled: user-approved — chosen over 一并挪到渠道能力表：保留精细化控制且改动面小)
- KTD5. **Endpoints 配置保留但降级为高级覆盖**，不删除。(session-settled: user-approved — chosen over 删除端点配置：协议族内具体格式（embedding/image/audio）选择与 path/transport 覆盖仍依赖它，普通用户由渠道类型自动推导不感知)
- KTD6. **渠道撤销声明时派生条目禁用并附原因，保留条目不删除**。(session-settled: user-directed — chosen over 静默删除：保留已调整的优先级，且被撤销的能力应可恢复)
- KTD7. **派生条目带自动标记（auto），派生逻辑永不修改或删除手动条目**。
- KTD8. **协议勾选交互为"渠道级多选协议 → 模型默认继承全部 → 逐个去掉例外"**。(session-settled: user-directed — chosen over 逐模型独立勾选：绝大多数模型与渠道能力一致，继承+例外最少操作)
- KTD9. **统一启用校验**：任何禁用态派生关联的启用（首次启用、批量启用、撤销后重新启用）都执行同一组校验（渠道存在且启用、渠道当前声明该模型与该协议、渠道端点具备该协议族完整格式、物理模型名可被渠道运行时模型条目解析），校验失败拒绝启用并提示具体原因。(session-settled: user-directed — chosen over 无校验直接启用：把"渠道实际不支持"的误配拦在配置阶段，且行为不因条目历史状态而异) 启用成功后条目转为手动条目，后续派生不再自动处置。
- KTD10. **统一状态机：派生条目一经用户启用即转手动条目**，脱离撤销对账（R6）处置范围；未被启用过的派生条目保持 auto、受对账管理。
- KTD11. **对账以实体为基准、双向核对**：撤销判定基于提交声明与既有声明的差异；撤销方向按 (渠道, 派生条目) 双向核对，不依赖逻辑模型当前名称；保存未触及的声明（含逐模型协议例外）必须原样保留（round-trip 保真）。
- KTD12. **auto_sync 引起的模型增删视为声明变更**：新增模型继承渠道当前勾选的全部协议并触发派生；移除模型触发撤销对账。
- KTD13. **auto 派生条目用户不可删除、只可禁用**；用户侧永久去除的正确途径是渠道撤销声明。唯一例外是渠道删除时的系统级清理（R13）。

### Requirements

**渠道能力声明**

- R1. 渠道编辑侧可多选该渠道支持的协议族；可选项限定为协议池 key 白名单（当前 openai/anthropic）。
- R1a. 声明与端点能力一致性：勾选协议族时校验渠道端点（含按类型推导的默认端点）具备该协议族的完整格式；不具备则拒绝勾选并提示原因。声明与端点冲突时以端点能力为准。
- R1b. 端点变更复核：渠道端点配置单独保存后，对既有声明按新端点能力复核；不再被端点支持的声明自动按撤销处理（触发撤销对账，R6）并提示。
- R2. 渠道添加模型（能力编辑器内：手动添加或拉取模型列表）时，新模型默认继承渠道当前勾选的全部协议；可对单个模型去掉某个协议。逐模型例外持久保存，渠道级协议取消后重新勾选不自动恢复曾有例外的模型的该协议。（实施层收窄见 KTD14：继承只发生在能力表写入面。）
- R2a. 渠道新勾选一个此前未声明的协议族时，既有已声明模型自动获得该协议声明并触发批量派生（与撤销方向对称）。
- R3. 渠道协议能力声明持久化保存，保存成功后触发派生对账（R4-R8）。保存未触及的模型/协议声明（含逐模型例外）必须原样保留；撤销判定基于提交声明与既有声明的差异，不做全量覆盖式比较。

**自动派生**

- R4. 派生规则：逻辑模型名与渠道声明的模型名精确相同时，在该逻辑模型的对应协议池中生成一条指向该渠道的关联，物理模型名取渠道声明的模型名，状态为禁用，带自动标记，初始优先级为 0（并列时沿用现有运行时排序语义）。
- R5. 判重基于有效关联视图（含 developer settings 继承的关联）：已存在同 (模型, 协议, 渠道) 的手动关联（任何层）时，派生不生成、不覆盖；已存在的派生关联保留用户调整过的优先级。
- R6. 渠道撤销某模型或某协议声明时，对应 **auto 派生条目**置为禁用并附禁用原因；条目保留，不删除。撤销对账只作用于 auto 条目：条目一经启用即转手动（KTD10），因此撤销不会禁用正在服务的关联，对账执行无需人工确认（含 auto_sync 定时触发的场景）。若被撤销的声明仍有手动条目（含转手动而来）指向，只提示"该条目已无渠道声明背书"，不做处置（手动条目不受派生逻辑触碰，KTD7）。
- R6a. 统一启用校验：任何禁用态派生关联的启用均须通过校验——渠道存在且状态启用、渠道当前声明该模型且支持该协议、渠道端点具备该协议族完整格式、物理模型名可被渠道运行时模型条目解析；任一不满足则拒绝启用并提示具体原因。校验通过并启用后，条目转为手动条目。
- R6b. 批量启用部分成功语义：逐条执行 R6a 校验，通过的启用、失败的跳过，结果展示失败清单及原因。
- R7. 渠道重新声明先前撤销的模型/协议时，仍处于"撤销禁用"状态的派生关联清除禁用原因、恢复为普通派生状态（仍为禁用，等待启用）；恢复后的条目再启用适用 R6a。
- R8. 派生对账幂等：重复保存渠道配置不产生重复关联，不改动手动条目，不重置已启用条目的启用状态与优先级。

**Model 侧**

- R9. Model 协议池自动展示派生关联；Model 侧操作限定为：启用/禁用、调整优先级、编辑关联（派生条目被编辑——含条件路由——即转为手动条目）、手动新增/删除关联。auto 派生条目不可删除、只可禁用（KTD13）。
- R10. 模型新建/编辑页提供"一键同步渠道"：按逻辑模型名查询所有声明了该模型名的渠道及其协议，在协议池中生成/刷新派生关联（默认禁用）。同步只做增量对账：复用 R5/R7/R8 的既有实体处置规则（保留优先级、对已重新声明的条目清除禁用原因），不重置已启用条目的状态，不执行撤销对账（撤销仅由渠道保存触发）。
- R11. 渠道保存后提示两类结果：①新增 N 个派生关联，附一键批量启用入口（走 R6b）；②M 个声明模型名未匹配到逻辑模型，附操作指引（创建逻辑模型后可用 R10 拉取）。

**渠道生命周期与自动同步**

- R12. auto_sync 同步引起的模型增删视为声明变更（KTD12）：新增模型继承渠道当前勾选的全部协议并触发派生对账；上游移除的模型触发撤销对账（R6，无需人工确认）。新增模型无同名逻辑模型时不派生，记入渠道侧"未匹配声明"清单并提示（同 R11 ②的指引）。
- R13. 渠道删除时：指向该渠道的 auto 派生条目随之清理；手动条目保留并置为禁用、附原因"渠道已删除"。渠道归档时：派生条目保留不做处置（运行时只选择启用渠道，归档渠道自动排除）；启用校验要求渠道状态为启用（R6a）。
- R14. 逻辑模型重命名不改变派生条目归属：条目随模型实体存在；撤销对账按 (渠道, 派生条目) 双向核对（KTD11），不依赖逻辑模型当前名称，重命名后的模型仍正常参与对账与路由。

**运行时与兼容**

- R15. 运行时路由读取 protocolPools 的逻辑不变；派生条目与手动条目在运行期语义完全一致。
- R16. 存量显式关联（含 developer 继承层）全部视为手动条目，原样生效，零数据迁移；派生逻辑只写模型级 settings，不触碰其他层。
- R17. 渠道协议声明与派生均受协议池 key 白名单约束（当前 openai/anthropic）；白名单本身本次不扩展。

### Key Flows

**F1. 渠道协议能力声明**
**Trigger:** 运维编辑渠道。
先多选协议族（openai/anthropic，勾选时校验端点能力）→ 添加模型（手动或拉取列表），模型默认继承全部已选协议 → 对不支持的模型逐个去掉协议 → 保存 → 触发派生对账 → 提示新增关联数量（一键启用入口）与未匹配声明数量（指引）。
**Covers R1, R1a, R1b, R2, R2a, R3, R4, R11.**

**F2. 一键同步渠道**
**Trigger:** 运维在模型新建/编辑页点击"一键同步渠道"。
按逻辑模型名查询渠道能力声明 → 为每个匹配渠道×协议生成或刷新派生关联（增量：保留已有条目状态与优先级，对重新声明的撤销禁用条目清除原因）→ 协议池 UI 刷新展示。不执行撤销对账。
**Covers R10, R4, R5, R7, R8.**

**F3. 撤销声明对账**
**Trigger:** 渠道保存的声明相对既有声明减少了某模型/协议（基于差异判定），或端点变更复核发现声明失去能力支撑（R1b）。
按 (渠道, 派生条目) 双向核对 → auto 条目禁用并写入原因（无需人工确认，因不影响已启用流量）→ 存在指向被撤销声明的手动条目时仅提示 → Model 协议池 UI 显示禁用+原因 → 用户尝试重新启用 → R6a 统一校验 → 通过则启用并转手动条目，失败则提示原因保持禁用。
**Covers R6, R6a, R7, R14, KTD6, KTD9, KTD10, KTD11.**

**F4. 自动同步联动**
**Trigger:** auto_sync 定时任务更新了渠道模型列表。
新增模型继承渠道当前全部协议声明 → 触发派生对账；移除模型 → 触发撤销对账（R6）。
**Covers R12, KTD12.**

### Acceptance Examples

- AE1. **新渠道接入**：渠道 kiro 声明 anthropic 支持 opus-4.7、openai 支持 gpt-5.6-luna 并保存。逻辑模型 opus-4.7 的 anthropic 池出现 kiro 派生关联（禁用、auto、优先级 0）；gpt-5.6-luna 的 openai 池出现 kiro 派生关联（禁用）；opus-4.7 的 openai 池不出现 kiro。保存后出现"新增 2 个关联"提示与一键启用入口；若声明中还包含平台不存在的模型名 x-1，提示"1 个声明模型未匹配逻辑模型"并给出指引。**Covers R1-R4, R11.**
- AE2. **撤销协议与重新启用**：模型 A 原声明支持 anthropic+openai，渠道更新去掉 anthropic。保存后模型 A 的 anthropic 池 auto 条目直接被禁用并显示原因（条目处于禁用态、无需确认；已启用条目必为手动、不受撤销影响）；openai 池不受影响。此时尝试重新启用被拒绝并提示"渠道当前不支持该协议"；渠道重新声明 anthropic 后该条目清除原因恢复为普通禁用态，重新启用通过校验、关联转为手动条目，后续派生不再自动禁用它。**Covers R6, R6a, R7, R14, KTD9, KTD10.**
- AE3. **自定义名称模型**：逻辑模型名与上游不一致时不出现任何派生关联；运维手动新增协议池并关联渠道+物理模型，行为与现状一致。**Covers R4, R9.**
- AE4. **存量兼容**：上线前已配置的显式关联（含 developer 继承层）全部按手动条目保留，派生对账不改动。**Covers R5, R16.**
- AE5. **幂等与优先级保留**：同一渠道连续保存两次，关联数量与状态不变；用户调高某派生条目优先级后，渠道声明变更触发对账，该优先级保持不变。**Covers R8, R5.**
- AE6. **一键同步增量性**：模型 B 已有启用的 kiro 派生条目与一条手动关联；对 B 执行一键同步渠道后，已启用条目状态与优先级不变，手动关联不被覆盖，不产生重复条目；若 kiro 曾撤销又重新声明 B 的某协议，对应撤销禁用条目被清除原因恢复。**Covers R10, R5, R7, R8.**
- AE7. **auto_sync 联动**：开启 auto_sync 的渠道上游新增模型 foo（平台存在同名逻辑模型）：下次同步后 foo 自动获得该渠道当前全部协议的派生关联（禁用）；上游随后下线 foo：再同步后对应派生关联被禁用并附原因。**Covers R12, KTD12.**
- AE8. **逻辑模型重命名**：逻辑模型 foo 持有渠道 C 的派生条目，重命名为 bar 后：渠道 C 保存撤销 foo 声明，bar 中该条目按 (渠道, 条目) 核对被禁用并附原因；渠道 C 仍声明 foo 时该条目继续正常路由。**Covers R14, KTD11.**
- AE9. **批量启用部分成功**：一键启用 5 条派生关联，其中 2 条对应渠道端点不具备该协议族格式：3 条启用成功，2 条跳过并在结果中列出原因。**Covers R6a, R6b, R11.**
- AE10. **渠道删除与归档**：删除（含批量删除路径）持有派生条目的渠道后，指向它的 auto 条目被清理，手动条目禁用并附原因"渠道已删除"，尝试启用被拒绝；归档渠道的条目保留但启用校验因渠道非启用状态被拒绝。**Covers R13, R6a.**
- AE11. **新协议族传播与例外存续**：渠道先声明模型集合于 openai，其中模型 M 被单独去掉 openai 例外保存；渠道随后新勾选 anthropic：既有模型（含 M）自动获得 anthropic 声明并批量派生；渠道再取消并重新勾选 openai 后，M 不自动恢复 openai 声明。**Covers R2, R2a.**
- AE12. **端点变更复核**：渠道已声明 anthropic 支持模型 A；运维在端点配置中移除 anthropic/messages 端点并保存。复核发现 anthropic 声明失去能力支撑，按撤销处理：auto 条目禁用并附原因，渠道侧提示该变更；手动条目仅提示不处置。**Covers R1b, R6.**

### Success Criteria

- 接入一个新渠道并声明能力后，若声明的模型名均已有同名逻辑模型，运维只需在提示处批量启用，无需进入任何模型编辑页即可完成接入；存在未匹配模型名时获得明确指引。
- 误配收敛到渠道声明一处并经过校验闸门：声明受端点能力校验（R1a/R1b），启用受统一能力校验（R6a）；通过校验启用的关联在运行时必然可命中。
- 存量配置与运行时路由行为零回归。

### Scope Boundaries

**In scope:** 渠道协议能力声明与存储、声明/启用校验、派生对账服务（含 auto_sync 联动、渠道删除/归档、模型重命名语义）、Model 侧派生条目展示与启停/优先级/编辑转手动、一键同步渠道按钮、一键批量启用入口、撤销声明的禁用+原因提示。

**Out of scope:**
- 新增协议族（gemini/ollama 等进入协议池白名单）——另开任务。
- Adapter 绑定结构变更（仍为 source_model_id -> model_id）。
- Endpoints 配置界面重构（仅定位调整为高级覆盖）。
- 存量显式关联的批量迁移或转换。
- regex/tags 类高级关联类型的行为变化。
- 事件驱动配置刷新（发布订阅）机制本身的建设——相邻独立工作，见 How This Work Fits Together。

### How This Work Fits Together

<!-- ce-section: work-relationships -->

- **本工作：渠道协议能力声明与协议池自动派生**——当前活跃工作单元。
- **事件驱动配置刷新（发布订阅）**——相邻、可独立规划的工作单元：配置更新（渠道/模型/Adapter）时在进程内发布事件，各订阅方自行完成副作用（Adapter 运行时快照刷新、派生对账、运行时缓存失效等）。关系为初步判断（tentative）：本计划的派生触发以显式调用实现（渠道保存、一键同步、auto_sync 联动三处）；事件机制落地后可迁移为订阅。事件机制本身不依赖本工作，可先行或并行立项。
- **新协议族接入（gemini/ollama 等）**——依赖白名单扩展，与本工作无直接依赖，另行立项。

### Dependencies / Assumptions

- 派生触发点三处：渠道保存、一键同步渠道（增量）、auto_sync 模型列表变更联动；均为显式实现，不假设存在自动刷新链路。
- 渠道已有拉取模型列表能力（Query `fetchModels`，`internal/server/biz/model_fetcher.go:237`）与 `supported_models` 同步机制（`internal/server/biz/channel_model_sync.go`，60 分钟周期），能力声明交互复用其结果。
- 运行时关联匹配要求物理模型名存在于渠道模型条目（`internal/server/biz/model_association_matcher.go`），启用校验（R6a）以此为据。
- 运行时有效协议池为模型级 settings 与 developer 继承的合并（`internal/server/biz/model_settings_inheritance.go:133`）；R5/R16 的判重与兼容以该有效视图为基准。
- 假设模型与渠道数量级下，保存时批量派生写入可在单事务内完成；分批策略由 ce-plan 处理。
- 现状备注：`UpdateAdapter` 保存后未调用快照 `Refresh`（`internal/server/biz/adapter.go:366-496`）；该问题的系统性修复见事件驱动配置刷新。

### Outstanding Questions

**Deferred to Planning:** 已由 ce-plan 解决，见 Planning Contract：
- 能力表数据形态 → KTD14（独立 JSON 字段 protocol_capabilities）
- 派生事务与并发 → KTD16 + Risks（单事务 merge-write，不引入乐观锁）
- 一键同步数据源 → KTD17（直查本地能力表，不调上游）
- 前端呈现 → KTD18/KTD20（payload 结构化统计、auto 徽章与原因展示）
- GraphQL 变更与生成 → KTD18（make generate，遵 ent-graphql.md）

**Resolve Before Planning:** 无。

### Sources / Research

- 协议池 key 白名单：`internal/objects/model.go:47-50`（SupportedInboundAPIFormats）。
- ModelSettings.ProtocolPools 与 ModelAssociation（Priority/Disabled/When）：`internal/objects/model.go:40-43,95-106`。
- 渠道 endpoints JSON 与合并解析：`internal/ent/schema/channel.go:153-159`、`internal/server/biz/channel_endpoint.go:206-265`。
- 渠道扁平模型列表（无协议维度）：`internal/ent/schema/channel.go:122-124`。
- 端点跨协议兜底选择：`internal/server/orchestrator/select_endpoints.go:71-121`。
- 运行时关联匹配与模型条目依赖：`internal/server/biz/model_association_matcher.go:98-176`、`internal/server/biz/channel_llm.go:1171-1240`。
- settings 继承与有效协议池合并：`internal/server/biz/model_settings_inheritance.go:133-155`、`internal/server/orchestrator/candidates.go:164-169`。
- 协议池编辑 UI 与渠道过滤守卫：`frontend/src/features/models/components/models-action-dialog.tsx`、`frontend/src/features/models/data/protocol-pools.ts:32-39`。
- 拉取模型列表：Query `fetchModels`（`internal/server/gql/model.graphql:224`）、`internal/server/biz/model_fetcher.go:237`。
- 模型列表自动同步（60 分钟周期、全量覆盖 supported_models）：`internal/server/biz/channel_internal.go:23-30`、`internal/server/biz/channel_model_sync.go:17-48,70-160`。
- 绑定规则现状约束：`.agent/rules/adapter-model-binding.md`（本计划对其中 Model-centric 条款构成一次经确认的架构演进，实现时需同步更新该文档）。

## Planning Contract

### Key Technical Decisions（实施层，续）

- KTD14. **能力存储为 Channel 上新增独立 JSON 字段 `protocol_capabilities`**，结构 `objects.ChannelProtocolCapabilities { DeclaredProtocols []string; Models []ChannelModelCapability{ModelID string; Protocols []string} }`；不改动 supported_models/manual_models 结构。**继承与派生触发只发生在能力表写入面**（saveChannelCapabilities、auto_sync 联动）；UpdateChannel/批量导入直接写 supported_models/manual_models 不进能力表、不触发继承。Governs R1-R3, R12, KTD11。理由：supported_models 是扁平列表，被运行时模型条目（GetModelEntries）、auto_sync 全量覆盖与多处 UI 消费，加协议维度波及面大；能力是正交元数据，逐模型物化协议天然满足 round-trip 保真与差异对账；收窄继承写入面避免 UpdateChannel/批量导入等多路径遗漏触发点。
- KTD15. **ModelAssociation 新增 `auto bool` 与 `disabledReason string`**（JSON omitempty），GraphQL ModelAssociation/Input 同步加字段。Governs R6, R9, KTD7/KTD10/KTD13。理由：复用现有 settings JSON 与 GraphQL 绑定；运行时忽略新字段自然兼容；存量数据无 auto 字段即 auto=false，按手动条目对待，恰满足 R16。
- KTD16. **派生引擎 = 纯函数 reconcile + 单 ent 事务**：新建 `internal/server/biz/protocol_pool_derivation.go`；`reconcileAssociations(新旧能力声明, 各模型现有 pools)` 产出操作清单（新增/禁用/恢复/清理），写层在事务内读最新 settings 做 merge-write（只触碰 auto 条目）；另提供按逻辑模型名反查能力表的**增量模式**（只新增/恢复、不撤销，服务 R10）与渠道删除清理函数（服务 R13）。不引入事件机制。Governs R4-R8, R10, R12-R14, KTD2/KTD10/KTD11。理由：状态机复杂，纯函数才能被单测穷举；单事务保证原子性；显式调用比隐式刷新可控（事件机制见 How This Work Fits Together）。
- KTD17. **一键同步渠道 = 新 mutation `deriveModelAssociations(modelID: ID!)`，只查本地能力表**，不调用上游 fetchModels。Governs R10。理由：派生只依赖已声明事实；上游拉取慢、需凭据且可能失败；前端 fetchModels 保持只服务渠道"拉取模型列表"场景。
- KTD18. **GraphQL 表面**：新增 `saveChannelCapabilities(input): SaveChannelCapabilitiesPayload!`（payload：channel、addedCount、unmatchedModels[]、revoked{autoDisabledCount, manualNotices[]}）、`bulkEnableDerivedAssociations(input): BulkEnableDerivedAssociationsResult!`（逐条成功/失败原因，R6b）；`saveChannelEndpoints` 返回类型改为 `SaveChannelEndpointsPayload{channel, revoked}`（破坏性变更，前端 hook 同步改，服务 R1b/AE12 的渠道侧提示）；Channel 增加 `protocolCapabilities` 字段（entgql.Skip + axonhub.graphql forceResolver 声明，避免 ent.graphql 自动生成造成重复字段）。schema 写在 `axonhub.graphql`，新结构体在 gqlgen.yml 映射到 objects.*，`make generate` 生成。Governs R1a/R1b/R6a/R6b/R11, F1/F2。
- KTD19. **启用校验的服务端钩子**：**仅 auto 派生条目的启用**（含首次启用、撤销禁用后重新启用）执行 R6a 校验，失败整体拒绝保存并返回结构化错误（渠道/模型/原因）；手动条目与新增手动关联豁免（保 AE3/KTD4 现状行为）；developer settings 写路径（SystemService）不在该闸门内。校验为带 ctx 的 svc 层方法（需查渠道/能力/端点/模型条目），覆盖 CreateModel/UpdateModel/BulkCreateModels 全部 settings 写入路径；入站 associations 的 auto/disabledReason 一律以服务端现存状态为准（忽略客户端提交值），且拒绝删除现存 auto 条目（KTD13 服务端强制）。批量启用走专用 mutation 返回部分成功。共享校验函数放 `internal/server/biz/model_capability_validation.go`，端点族判断与前端 `channelSupportsProtocolPool` 同语义。Governs R1a/R6a/R6b, R9/KTD13 强制, AE9。理由：启用最终落为 settings 写入，服务端拦截是唯一可靠强制点；校验对象限定 auto 条目避免回归存量手动配置行为。
- KTD20. **前端形态**：能力编辑器为新独立对话框 `channels-capability-dialog.tsx`（行菜单入口，与 Endpoints 对话框同级）：协议族多选 → 模型候选（supported_models∪manual_models，可 fetchModels 补充）默认继承 → 逐模型去掉；models-action-dialog 展示 auto 徽章、disabledReason、auto 条目隐藏删除、"同步渠道"按钮与启用失败提示。Governs F1/F2, R2/R9/R11, KTD8。

### Technical Design（要点）

- **数据流**：能力保存 → reconcile → 写各 model.settings.protocolPools（auto 条目）；请求路由链路（candidates.go:164）不修改（R15）。ModelService 无缓存，派生写入即时生效；ChannelService 运行时渠道缓存不含模型 settings，无需失效处理。
- **端点族判断**：`channelSupportsProtocolFamily(resolvedEndpoints, family)` 按前缀语义与前端 `protocolPoolEndpointApiFormatPrefixes` 一致（openai→`openai/`，anthropic→`anthropic/`），R1a/R1b/R6a 共用。
- **迁移**：新 JSON 字段由 ent 启动自动迁移处理（`internal/server/db/ent.go` 的 `client.Schema.Create`，WithDropColumn 已开），字段声明加 entgql.Skip 避免进入 ent.graphql 与 Create/UpdateChannelInput；旧渠道空能力即现状行为，无需数据回填、无需手写 SQL。
- **渠道复制语义**：DuplicateChannel 不复制能力声明（能力是渠道账号×上游的客观事实，新渠道需重新声明），避免静默复制而无派生。
- **并发**：事务内 read-modify-write merge；SQLite 单写者竞争窗口小；不引入乐观锁（已知限制：极端并发下后写覆盖，merge 只触碰 auto 条目降低损失面）。
- **触发点装配**：saveChannelCapabilities（R3/R6/R7/R11）、SaveChannelEndpoints 后（R1b 复核，撤销统计经 payload 返回）、channel_model_sync.go 同步成功处（R12）、DeleteChannel 与 BulkDeleteChannels（R13 清理，共用 U4 清理函数）、deriveModelAssociations（R10 增量）。

### Risks

- reconcile 状态机复杂（auto/manual、撤销/恢复/转手动）→ 纯函数单测穷举 R4-R8 与 R12-R14 全部分支（见 Verification Contract）。
- settings JSON merge 与并发模型编辑 → 事务内读最新再合并；不引入版本号。
- 生成链失误 → 凡改 schema 必须 `make generate`；禁止手改 `internal/ent/` 与 gqlgen 生成文件（ent-graphql.md）。
- bulkEnable 校验涉及多渠道加载 → 一次查询加载涉及的全部渠道与模型条目，避免 N+1。

### Sequencing

U1 → U2/U4（可并行）→ U3 → U5 → U6 → U7 → U8/U9（可并行）→ U10。（U3 依赖 U4 的 reconcile 接口签名，故 U4 先于 U3。）

## Implementation Units

- U1. **objects 层：能力类型与关联新字段**。Modify: `internal/objects/channel.go`（ChannelProtocolCapabilities/ChannelModelCapability）、`internal/objects/model.go`（ModelAssociation 加 auto/disabledReason；SupportedInboundAPIFormats 白名单暴露共享校验函数）。Test: 新建 `internal/objects/model_capability_test.go`（JSON round-trip、auto 缺省 false、白名单校验）。Accept：现有测试全绿。Depends: 无。
- U2. **ent schema 与迁移**。Modify: `internal/ent/schema/channel.go`（`field.JSON("protocol_capabilities", objects.ChannelProtocolCapabilities{}).Default(...).Optional().Annotations(entgql.Skip(entgql.SkipAll))`——字段完全不进 ent.graphql（类型与 mutation input 均不生成），对齐 KTD18：GraphQL 面由 axonhub.graphql 声明，唯一写入面为 saveChannelCapabilities）。Run: `make generate`；enttest 验证字段读写与自动迁移。Depends: U1。
- U3. **能力服务：存储、端点校验与生命周期钩子**。Create: `internal/server/biz/channel_capability.go`（SaveProtocolCapabilities 含 R1a 端点校验 + R17 白名单校验/差异计算/触发 reconcile/R11 统计、`channelSupportsProtocolFamily`、R1b 复核）。Modify: `internal/server/biz/channel.go`（SaveChannelEndpoints:867 后调 R1b 复核并返回含 revoked 的 payload；DeleteChannel 调 R13 清理）、`internal/server/biz/channel_bulk.go`（BulkDeleteChannels 同样接 R13 清理）、`internal/server/biz/channel_model_sync.go`（syncChannelModelsForChannel:70 的 modelsChanged 分支：addedModels 继承渠道当前协议声明并触发派生、removedModels 触发撤销对账，R12/KTD12）。Test: `internal/server/biz/channel_capability_test.go`（enttest：R1a 拒绝、R17 拒绝非法 key、R3 round-trip 保真、差异判定、R2a 新协议传播）。Depends: U1 U2。
- U4. **派生对账引擎**。Create: `internal/server/biz/protocol_pool_derivation.go`（纯函数 reconcileAssociations + 事务 apply；含 R5 判重（含 developer 继承有效视图）、R6/R7 撤销恢复、R12 同步语义、R13 渠道删除清理函数、R14 按 (渠道,条目) 核对、**R10 按逻辑模型名反查能力表的增量模式（只新增/恢复不撤销）**）。Test: `internal/server/biz/protocol_pool_derivation_test.go`（表驱动穷举 R4-R8/R10/R12-R14 与 KTD10 状态机）。Depends: U1。
- U5. **启用校验与 Model 钩子**。Create: `internal/server/biz/model_capability_validation.go`（带 ctx 的 svc 层方法，R6a 共享校验：渠道存在且启用、声明存在、端点族、物理模型条目可解析）。Modify: `internal/server/biz/model.go`——在 CreateModel(:347)/UpdateModel(:444)/BulkCreateModels(:383) 三个 settings 写入路径统一接入；UpdateModel 校验前加载现存模型 settings 做 disabled→enabled diff；**仅对 auto 派生条目的启用生效，手动条目与新增手动关联豁免（保 AE3/KTD4）**；入站 associations 的 auto/disabledReason 以服务端现存状态为准（忽略客户端提交值），拒绝删除现存 auto 条目（KTD13 服务端强制）。ModelService 已持有 channelService/systemService（:48-49）可直接依赖。Test: `internal/server/biz/model_validation_test.go` 扩展（AE9 拒绝与原因、BulkCreateModels 不绕过、存量手动行为不变）。Depends: U1 U3。
- U6. **GraphQL 层**。Modify: `internal/server/gql/axonhub.graphql`（saveChannelCapabilities、bulkEnableDerivedAssociations、deriveModelAssociations 与 payload 类型；saveChannelEndpoints 返回改 SaveChannelEndpointsPayload；Channel 的 protocolCapabilities 用 `@goField(forceResolver: true)` 在 `extend type Channel` 声明——**因 U2 已 entgql.Skip，不会与 ent.graphql 自动生成冲突**）、`internal/server/gql/model.graphql`（ModelAssociation auto/disabledReason）、`internal/server/gql/gqlgen.yml`（ChannelProtocolCapabilities/ChannelModelCapability(+Input) 映射到 internal/objects.*，对齐 ChannelEndpoint 模式）。Run: `make generate` 后实现新 resolvers（axonhub.resolvers.go/model.resolvers.go）。Test: resolver 单测。Depends: U3 U4 U5。
- U7. **前端：渠道能力编辑器**。Create: `frontend/src/features/channels/components/channels-capability-dialog.tsx`。Modify: `channels-columns.tsx`（行菜单）、`channels-context.tsx`（ChannelsDialogType 加 'capability' 成员）、`channels-dialogs.tsx`（挂载 ChannelsCapabilityDialog，参照 endpoints 写法）、`data/channels.ts`（useSaveChannelCapabilities/useBulkEnableDerivedAssociations；**onSuccess invalidate ['channels']、['channel', id] 与 ['models']**——派生写入的是各 Model settings）、`data/schema.ts`（zod；协议白名单跨 feature import 自 `@/features/models/data/protocol-pools` 的 protocolPoolFormats）、`frontend/src/locales/{en,zh-CN}/channels.json`。交互：KTD8 流程；保存后展示 addedCount/unmatchedModels 与一键启用入口。Test: node:test（`node --test`，沿用 protocol-pools.test.mjs 模式；组件无法在 node:test 渲染，只测纯构造/校验逻辑，组件行为靠浏览器验收）。Depends: U6。
- U8. **前端：Model 侧派生条目**。Modify: `frontend/src/features/models/components/models-action-dialog.tsx`（auto 徽章、disabledReason、auto 条目隐藏删除、启用失败 toast、"同步渠道"按钮）、`data/models.ts`（useDeriveModelAssociations；**onSuccess invalidate ['models'] 及模型详情相关 key**）、`data/schema.ts`、`locales/{en,zh-CN}/models.json`。Depends: U6 U7。
- U9. **规则与文档更新**。Modify: `.agent/rules/adapter-model-binding.md`（渠道能力声明体系与 Model-centric 条款的新边界）、`docs/architecture/backend.md`、`docs/architecture/frontend.md` 相关章节。Depends: U6-U8 定型后。
- U10. **E2E 验收**。按 `.agent/rules/e2e.md` 用 curl/浏览器验收 AE1-AE12（重点 AE1/AE2/AE9/AE12）；产出持久验收产物（curl 记录/截图存 `.agent/summary/<feature>/`）；若涉 Docker 静态资源按其约定重建；写功能总结到 `.agent/summary/`。Depends: 全部。

## Verification Contract

- **单元测试**：各 Unit 测试文件如上；对账引擎覆盖 R4-R8/R12-R14 全部条款与 KTD10 状态机（含 AE2 撤销→重新启用双向序列）；启用校验覆盖 AE9 部分成功。
- **生成验证**：`make generate` 通过；`internal/ent/` 与 gqlgen 产物无手改。
- **回归**：主模块 `go test ./internal/...` 全绿；orchestrator 运行时路径零改动（R15，candidates.go 不在修改清单）；**backup/restore 对新字段的往返回归**（internal/server/backup/restore.go 处理 protocolPools，需验证 auto/disabledReason 不丢失、不破坏导入）。
- **前端**：`npm run test:unit`（node --test）全绿；能力对话框与派生条目展示按 e2e.md 浏览器验收。
- **AE 追溯**：AE1→U3/U4/U7；AE2→U4/U5/U8；AE3→U4/U5/U8；AE4→U4/U5；AE5→U4；AE6→U4/U6/U8；AE7→U3/U4；AE8→U4；AE9→U5/U6/U7；AE10→U3/U5；AE11→U3/U4/U7；AE12→U3/U6。

## Definition of Done

- Verification Contract 全绿，AE1-AE12 通过 E2E 验收。
- `.agent/rules/adapter-model-binding.md` 与架构文档已同步；`.agent/summary` 有本功能总结。
- 无手改生成文件；无 credentials/真实代理地址进入代码、文档、日志（项目规则）。
