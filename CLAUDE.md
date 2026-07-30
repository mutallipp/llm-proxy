# llm-proxy 开发指南

@AGENTS.md

## 项目定位

llm-proxy 是基于 AxonHub 核心能力维护的统一 AI 网关，提供多协议转换、渠道路由、模型组和 Adapter 入口。
内部 Go module、包路径和 `AXONHUB_*` 环境变量暂时保持兼容，不要因为产品改名直接批量重命名内部标识。

## 文档入口

- [项目 README](README.md)
- [后端架构](docs/architecture/backend.md)
- [前端架构](docs/architecture/frontend.md)
- [中文开发指南](docs/zh/development/development.md)
- [Docker 部署](docs/zh/deployment/docker.md)
- [Adapter MVP 部署说明](docs/deployment/adapter-mvp.md)

## 开发约定

1. 修改前先阅读调用方、数据结构、配置和相邻测试。
2. 前后端同时修改时先划分文件 ownership 和共享契约，再分别委派 Agent。
3. GraphQL、REST、数据库和运行时对象之间新增字段时必须完成全链路映射。
4. 配置变更后同时验证持久化配置和运行时快照，不要只看数据库。
5. 涉及前端交互时用浏览器 snapshot、Network、Console 和截图验收。
6. 不把 credentials、真实代理地址或临时密码写入代码、文档、日志或提交。

## Adapter / ModelGroup 关键约定

- Adapter 入站协议固定；ModelGroup 可按入站协议维护独立目标池。
- Target 由 `Channel + targetModelId + outbound_api_format` 组成。
- GraphQL Channel 全局 ID 传 REST numeric `channel_id` 前必须用 `extractNumberID` 转换。
- 目标草稿点击“添加”后才进入目标池，目标池为空时不能保存有效路由。
- `capabilities.stream_policy` 使用 `unlimited`、`require`、`forbid`；缺省按跟随下游处理。
- Adapter 配置保存后应刷新运行时 snapshot；出现 `no usable target` 时优先检查绑定、协议、目标、渠道状态和 snapshot。
