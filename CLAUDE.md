# llm-proxy 开发指南

@AGENTS.md

## 项目定位

llm-proxy 是基于 AxonHub 核心能力维护的统一 AI 网关，提供多协议转换、渠道路由、模型组和 Adapter 入口。
内部 Go module、包路径和 `AXONHUB_*` 环境变量暂时保持兼容，不要因为产品改名直接批量重命名内部标识。

## 文档入口

- [项目 README](README.md)
- [后端架构与 Model-centric Adapter Gateway](docs/architecture/backend.md)
- [前端架构与 Model 协议池页面](docs/architecture/frontend.md)
- [中文开发指南](docs/zh/development/development.md)
- [Docker 部署](docs/zh/deployment/docker.md)
- [Adapter MVP 部署、迁移与回滚](docs/deployment/adapter-mvp.md)
- [Adapter/Model 绑定规则](.agent/rules/adapter-model-binding.md)
- [定向测试与 E2E 规则](.agent/rules/e2e.md)

## 开发约定

1. 修改前先阅读调用方、数据结构、配置和相邻测试。
2. 前后端同时修改时先划分文件 ownership 和共享契约，再分别委派 Agent。
3. GraphQL、REST、数据库和运行时对象之间新增字段时必须完成全链路映射。
4. 配置变更后同时验证持久化配置和运行时快照，不要只看数据库。
5. 涉及前端交互时用浏览器 snapshot、Network、Console 和截图验收。
6. 不把 credentials、真实代理地址或临时密码写入代码、文档、日志或提交。

## Model-centric Adapter Gateway（2026-08-02）

- Adapter 只绑定逻辑 Model（`source_model_id -> model_id`）；Model 的 `settings.protocolPools` 按入站协议族维护 Channel 和物理模型关联。
- 协议池 key（`openai`/`anthropic`）与 Channel endpoint 的完整 `apiFormat` 分开维护；禁止跨协议兜底。详细规则见 `.agent/rules/adapter-model-binding.md`。
- GraphQL Relay GID 在 Select 中保留原值，写入数值字段前使用 `extractNumberID`；保存后必须刷新 runtime snapshot。
- 旧 ModelGroup 仅属于迁移/drop gate，不再作为运行时或管理入口。
