# AGENTS.md

This file provides guidance to AI coding assistants when working with code in this repository.

> **Detailed rules are split into focused files under `.agent/rules/`**. See [Rules Index](#rules-index) below.

## Global Rules

1. Do NOT run lint or build commands unless explicitly requested by the user.
2. Do NOT restart the development server — it's already started and managed.
3. All summary files should be stored in `.agent/summary` directory if available.

### 开发约定

1. 修改前先阅读调用方、数据结构、配置和相邻测试。
2. 前后端同时修改时先划分文件 ownership 和共享契约，再分别委派 Agent。
3. GraphQL、REST、数据库和运行时对象之间新增字段时必须完成全链路映射。
4. 配置变更后同时验证持久化配置和运行时快照，不要只看数据库。
5. 涉及前端交互时用浏览器 snapshot、Network、Console 和截图验收。
6. 不把 credentials、真实代理地址或临时密码写入代码、文档、日志或提交。

## Configuration

- Backend API: port 8090, Frontend dev server: port 5173 (proxies to backend).
- Configuration: `conf/conf.go` (YAML + env var), SQLite by default.

## Project Overview

AxonHub is an all-in-one AI development platform that serves as a unified API gateway for multiple AI providers. It provides OpenAI and Anthropic-compatible API interfaces with automatic request transformation, enabling seamless communication between clients and various AI providers through a sophisticated bidirectional data transformation pipeline.

### 项目定位

llm-proxy 是基于 AxonHub 核心能力维护的统一 AI 网关，提供多协议转换、渠道路由、模型组和 Adapter 入口。
内部 Go module、包路径和 `AXONHUB_*` 环境变量暂时保持兼容，不要因为产品改名直接批量重命名内部标识。

## Technology Stack

- **Backend**: Go 1.26.0+ with Gin, Ent ORM, gqlgen, FX
- **Frontend**: React 19 + TypeScript, TanStack Router/Query, Zustand, Tailwind CSS

## Backend Structure

- `cmd/axonhub/main.go` — Application entry point
- `internal/server/` — HTTP server and route handling with Gin
- `internal/server/biz/` — Core business logic and services
- `internal/server/api/` — REST and GraphQL API handlers
- `internal/server/gql/` — GraphQL schema and resolvers
- `internal/ent/` — Ent ORM for database operations
- `internal/ent/schema/` — Database schema definitions
- `internal/contexts/` — Context handling utilities
- `internal/pkg/` — Shared utilities (xerrors, xjson, xcache, xfile, xcontext, etc.)
- `internal/scopes/` — Permission system with role-based access control
- `llm/` — LLM utilities, transformers, and pipeline processing (separate Go module)
- `llm/pipeline/` — Pipeline processing architecture
- `conf/conf.go` — Configuration loading and validation

## Go Modules

- The repository root (`/`) is the main Go module: `github.com/mutallipp/llm-proxy`.
- `llm/` is a separate Go module: `github.com/mutallipp/llm-proxy/llm`.

### `llm/` Module Notes

- `llm/` is an independent module. Always run Go commands from the `llm/` directory (e.g., `cd llm && go test ./...`).
- Running `go test ./llm/...` from repo root will fail with module boundary errors.

## Frontend Structure

- `frontend/src/routes/` — TanStack Router file-based routing
- `frontend/src/gql/` — GraphQL API communication
- `frontend/src/features/` — Feature-based component organization
- `frontend/src/components/` — Reusable shared components
- `frontend/src/hooks/` — Custom shared hooks
- `frontend/src/stores/` — Zustand state management
- `frontend/src/locales/` — i18n support (en.json, zh.json)
- `frontend/src/lib/` — Core utilities (API client, i18n, permissions, utils)
- `frontend/src/utils/` — Domain-specific utilities (date, format, error handling)
- `frontend/src/config/` — App configuration
- `frontend/src/context/` — React context providers

## 文档入口

- [项目 README](README.md)
- [后端架构、请求链路与 Model-centric Adapter Gateway](docs/architecture/backend.md)
- [前端架构、页面、GraphQL 与 Model 协议池页面](docs/architecture/frontend.md)
- [中文开发指南](docs/zh/development/development.md)
- [Docker 部署](docs/zh/deployment/docker.md)
- [Adapter/Model 绑定规则](.agent/rules/adapter-model-binding.md)
- [定向测试、curl、浏览器验收与 E2E 规则](.agent/rules/e2e.md)

## Rules Index

All detailed rules are in `.agent/rules/`:

| File | Scope | Description |
|------|-------|-------------|
| [go-general.md](.agent/rules/go-general.md) | `**/*.go` | Go 通用约定、错误处理、依赖注入、开发命令约束 |
| [ent-graphql.md](.agent/rules/ent-graphql.md) | `internal/ent/schema/**/*.go`, `internal/server/gql/**/*.go`, `internal/server/gql/**/*.graphql`, `gqlgen.yml` | Ent、GraphQL、代码生成、schema 变更规则 |
| [biz-services.md](.agent/rules/biz-services.md) | `internal/server/biz/**/*.go` | Biz service、上下文取值、事务与级联删除规则 |
| [cache-compat.md](.agent/rules/cache-compat.md) | `**/*.go` | 缓存结构兼容性与升级安全规则 |
| [frontend-general.md](.agent/rules/frontend-general.md) | `frontend/**/*.ts`, `frontend/**/*.tsx` | 前端通用开发约定、GraphQL 数据约束、页面作用域 |
| [frontend-i18n.md](.agent/rules/frontend-i18n.md) | `frontend/src/**/*.ts`, `frontend/src/**/*.tsx`, `frontend/src/locales/*.json` | i18n 与货币格式规则 |
| [frontend-ui.md](.agent/rules/frontend-ui.md) | `frontend/**/*.tsx` | 前端 UI 组件使用规则 |
| [adapter-model-binding.md](.agent/rules/adapter-model-binding.md) | Adapter/Model binding 相关前后端文件 | Model-centric binding、协议池 key、endpoint 边界和运行时刷新规则 |
| [e2e.md](.agent/rules/e2e.md) | `frontend/tests/**/*.ts`, `scripts/e2e/**/*.sh` | E2E 测试、定向验证和本地凭据规则 |
| [docs.md](.agent/rules/docs.md) | `docs/**/*.md` | Documentation rules |
| [workflows/add-channel.md](.agent/rules/workflows/add-channel.md) | Manual | Workflow for adding a new channel |

## Model-centric Adapter Gateway（2026-08-02）

- Adapter 只绑定逻辑 Model（`source_model_id -> model_id`）；Model 的 `settings.protocolPools` 按入站协议族维护 Channel 和物理模型关联、优先级和启用状态。
- 协议池 key（`openai`/`anthropic`）与 Channel endpoint 的完整 `apiFormat` 分开维护；禁止跨协议兜底。详细边界见 [adapter-model-binding.md](.agent/rules/adapter-model-binding.md)。
- GraphQL Relay GID 在 Select 中保留原值，写入数值字段前使用 `extractNumberID`；配置保存后必须刷新 runtime snapshot。
- 出现 `no protocol pool`、`model not bound` 或 `no usable target` 时，按架构文档和 E2E 规则逐层排查。
- 旧 ModelGroup 仅属于迁移/drop gate，不再作为运行时或管理入口。

### 子代理角色分工

- `frontend-worker`：实现前端功能；开始前声明允许/禁止修改的文件。
- `frontend-reviewer`：只做前端改动审查和验证，不与 worker 并发修改同一文件。
- `claude-code`：承担后端或全栈实现；共享契约由单一 owner 维护。
- 每个功能使用独立 worktree；集成 worktree 只做 cherry-pick 合并。子代理完成后必须报告变更文件和验证证据。
