---
title: Model-centric Adapter Gateway 验收报告
type: acceptance-report
date: 2026-08-02
---

# Model-centric Adapter Gateway 验收报告

## 1. 验收概述

本次集成将网关从 **ModelGroup 中心架构**迁移为：

```text
Model → ProtocolPools[inbound protocol] → Channel
```

Adapter 只保存 `model_id` binding，并以自身固定的 inbound protocol 选择 Model 内对应的协议池；协议池同时决定可用 Channel endpoint 和出站协议。运行时不再跨协议转换，也不再读取 ModelGroup 旧链路。

当前集成分支 HEAD：`d603b134`；U1 完成基线：`a4d8f10f`。

## 2. Unit 完成摘要

### U1：协议池契约

- 以 Model 为唯一逻辑模型中心，复用 `ModelSettings.Associations` 承载 `protocolPools`。
- 协议池 key 是 inbound protocol 和出站 endpoint 的唯一来源；target 不再保存 `outboundApiFormat`。
- 完成状态：✅ 已完成，基线提交 `a4d8f10f`。

### U2：数据迁移

- 将旧 ModelGroup 协议、target 和 Adapter binding 确定性回填到 Model 协议池及 `model_id` binding。
- 迁移按 logical Model 原子执行，支持冲突报告、幂等重试和失败 atomic abort。
- 完成状态：✅ 已完成，主要提交范围 `9ac04fbd..221caa71`，跨方言/事务/SQLite fixture 修复包含 `873743e8..f6339e46`。

### U3：binding 切换

- Adapter binding 从 `model_group_id` 切换为 `model_id`，移除 Channel、physical model 和 outbound 字段依赖。
- 支持不同 inbound protocol 的多个 Adapter 共享同一 Model。
- 完成状态：✅ 已完成，主要提交范围 `bb223ae5..8f968152`。

### U4：runtime 协议隔离

- snapshot 从 binding 取得 Model，再按 Adapter 固定 inbound protocol 选择同 key 协议池。
- 池内按 priority、enabled 和健康状态执行 failover；不再跨协议选择或转换。
- 完成状态：✅ 已完成，主要提交范围 `d8d98763..c85af528`，selector 类型边界及协议隔离修复在后续验收提交中完成。

### U5：前端协议池 UI

- `/models` 以协议池作为编辑边界，按 Channel `endpoints/defaultEndpoints` 过滤可用渠道。
- Adapter 绑定选择已有共享 Model，保存池内 Channel、physical model、priority 和 enabled 配置。
- 完成状态：✅ 已完成，主要提交范围 `5d0e253d..af15f75d`，并包含 Channel GID、模型 GID、React loop 等回归修复。

### U6：一刀切删除 ModelGroup

- 移除 ModelGroup 产品面、REST/GraphQL API、UI、路由、运行时读取、Ent schema/生成引用及旧表删除遗留。
- 迁移成功后才执行旧结构删除；不保留双写、legacy reader 或旧运行时兼容路径。
- 完成状态：✅ 已完成，提交范围 `09c94038..3a173920`。

### U7：验证修复

- 补齐 Adapter 刷新结果契约、APIFormat 类型边界、migration driver gate 和前端/运行时回归。
- 完成 SQLite migration gate、curl E2E、浏览器 E2E 及协议隔离验收。
- 完成状态：✅ 已完成，主要提交范围 `ca8417a5..d603b134`。

> 提交按主要变更归属列示；跨 Unit 的修复提交可能在 U7 中再次引用，以保留验收闭环。

## 3. 前端修复清单

- **Channel GID**：修复 Relay GID 被错误转换为数字导致协议池渠道选择丢失的问题。
- **Model GID**：修复 Adapter 创建/绑定页面将 Model Relay GID 转换为 `NaN`，导致绑定校验失败的问题。
- **React loop**：稳定模型列表引用，修复打开 Adapter 新建弹窗时的无限更新循环和 React error #185。
- **列表布局**：移除 Adapter 列表中已下线的 binding 详情区块，保持列表与新 binding 契约一致。
- **typecheck 回归**：完成本次改动相关类型修复；剩余 175 条 TypeScript 错误为既有问题，未发现本次新增回归。

## 4. 后端修复清单

- **AdapterRefreshResult**：补齐刷新结果契约，并修正字段类型使其与运行时 snapshot 一致。
- **APIFormat 类型**：收紧 `selectModelCandidates` 及 selector 测试 helper 的协议类型边界，避免字符串与协议类型混用。
- **protocol pool key 归一化**：统一协议池 key 的规范化，确保 Adapter inbound protocol 能稳定命中对应池。
- **privacy context**：修复 Adapter 启动刷新过程中的隐私上下文阻塞；general settings/retry policy 的 `no user in context` 告警不属于 Adapter 路径。
- **migration driver gate**：补齐 Ent migration driver 调用和 beta7/drop gate 顺序，确保迁移校验通过后才删除旧结构并启动新运行时。

## 5. 验证矩阵

| 验证项 | 结果 | 结论 |
|---|---:|---|
| Go 定向测试 | ✅ 通过 | 协议池、selector、snapshot、迁移和删除 gate 覆盖通过 |
| 前端 typecheck | ✅ 无新增回归 | 仍有 175 条既有 TypeScript 错误 |
| SQLite 迁移 gate | ✅ 通过 | beta7 + drop gate 自动执行，失败边界保持 fail-fast/atomic abort |
| curl E2E | ✅ 通过 | Adapter 管理、刷新和消费链路可用 |
| 浏览器 E2E | ✅ 通过 | Model 协议池配置、Adapter 绑定和页面回归通过 |
| 协议隔离 | ✅ 通过 | OpenAI Adapter 只命中 OpenAI pool，Anthropic Adapter 只命中 Anthropic pool |

## 6. 已知限制与后续项

- PostgreSQL 本次未进行真实连接验证，后续需在部署环境补充迁移和启动验收。
- MySQL 按设计 fail-fast，不作为本次兼容运行路径；后续若支持需单独设计方言迁移方案。
- 前端保留 175 条既有 TypeScript 错误，后续可独立清理，不能归因于本次迁移。
- general settings/retry policy 可能继续输出 `no user in context` 告警；已确认不在 Adapter 请求路径内，后续可单独治理告警上下文。
- 后续仍需补充负向协议、超时、多目标故障转移压测，以及 PostgreSQL 实连验证。

## 7. 部署变更说明

- Adapter binding 字段从 `model_group_id` 改为 `model_id`。
- 旧 ModelGroup API、UI、路由和运行时依赖已移除；客户端和管理端统一使用 Model + 协议池架构。
- 迁移由 `datamigrate beta7 + drop gate` 自动执行：先在迁移 gate 内完成 Model 协议池和 Adapter binding 回填及一致性校验，再删除旧表/约束/索引和旧生成引用；任何失败均阻止新运行时启动。
- 部署后请确认 Adapter 的每个 binding 指向有效 Model，Channel 的 `endpoints/defaultEndpoints` 包含对应协议池 key；不得通过旧 ModelGroup API 或旧字段配置。
