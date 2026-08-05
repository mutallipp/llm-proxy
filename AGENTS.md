# AGENTS.md

This file provides guidance to AI coding assistants when working with code in this repository.

> **Detailed rules are split into focused files under `.agent/rules/`**. See [Rules Index](#rules-index) below.

## Global Rules

1. Do NOT run lint or build commands unless explicitly requested by the user.
2. Do NOT restart the development server — it's already started and managed.
3. All summary files should be stored in `.agent/summary` directory if available.
4. **dev / prod 启停、构建、部署、日志统一走 Makefile target**。禁止在 `scripts/` 下新增 `start.sh` / `stop.sh` / `dev-*.sh` 之类的包装脚本（这类脚本与 Makefile 二选一，不要两边都有）。需要直调 docker compose 时也走 `make` 包装的 target，不要裸调 `docker compose`。

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

### 环境变量命名约定

- 当前生效前缀：`LLM_PROXY_`（2026-08 起硬切换，旧 `AXONHUB_` 已彻底移除，无向后兼容）
- 常见示例：`LLM_PROXY_DB_DSN`、`LLM_PROXY_DB_DIALECT`、`LLM_PROXY_SERVER_PORT`、`LLM_PROXY_SERVER_HOST`、`LLM_PROXY_SERVER_API_AUTH_ALLOW_NO_AUTH`、`LLM_PROXY_LOG_LEVEL`、`LLM_PROXY_HTTP_PROXY`
- 配置优先级仍按现有规则：环境变量 > 配置文件 > 默认值
- 详细字段定义见 `conf/conf.go` 和 `config.example.yml`

## Project Overview

llm-proxy is an all-in-one AI development platform that serves as a unified API gateway for multiple AI providers. It provides OpenAI and Anthropic-compatible API interfaces with automatic request transformation, enabling seamless communication between clients and various AI providers through a sophisticated bidirectional data transformation pipeline.

### 项目定位

llm-proxy 是基于原 AxonHub 核心能力维护的统一 AI 网关（前身名为 AxonHub，2026-08 完成品牌切换）。提供多协议转换、渠道路由、模型组和 Adapter 入口。

### 命名一致性

- **环境变量 / 数据库 / 部署配置 / 产品名**：已统一为 `llm-proxy` / `LLM_PROXY_*`
- **Go module 路径 / 包导入路径**：保持 `github.com/mutallipp/llm-proxy`（与 GitHub 仓库名一致；如需整体改为 `llmproxy` 等无连字符名以避免 gqlgen 生成函数名包含连字符，需同步重新生成 gqlgen 代码，本次未做）
- **GraphQL Relay GID 命名空间**：保持 `gid://axonhub/<Type>/<id>` 格式（数据库内已存大量该格式的 ID，切换需数据迁移，本次未做）

## Technology Stack

- **Backend**: Go 1.26.0+ with Gin, Ent ORM, gqlgen, FX
- **Frontend**: React 19 + TypeScript, TanStack Router/Query, Zustand, Tailwind CSS

## Backend Structure

- `cmd/llm-proxy/main.go` — Application entry point
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

## 本地开发与部署脚本

启动命令统一在 `Makefile` 里（inline docker compose，无需额外脚本文件）：

| 命令 | 说明 |
|---|---|
| `make start` | 构建 + 启动 prod 容器（端口 8090，使用 `docker-compose.yml`） |
| `make stop` | 停止 prod 容器（保留镜像） |
| `make restart` | 重新创建 prod 容器 |
| `make logs` | tail prod 容器日志 |
| `make dev-up` | 构建 + 启动 dev 容器（端口 18090，使用 `docker-compose.dev.yml` + `.env.dev`） |
| `make dev-down` | 停止并删除 dev 容器 |
| `make dev-logs` | tail dev 容器日志 |
| `make dev-restart` | 重新创建 dev 容器 |
| `make dev-clean` | 停止 dev + 删除 dev 镜像 |
| `make dev-frontend` | 本地起 Vite dev（端口 15173，代理到 18090） |
| `make dev` | 一键：起 dev 后端 + 前台跑 Vite（Ctrl+C 退出前端后需 `make dev-down` 停后端） |

dev 与 prod 状态完全隔离：dev DB 库名为 `llm-proxy-dev`（独立库），dev 容器名为 `llm-proxy-dev`，互不冲突。dev 首次启动前需手动 `CREATE DATABASE "llm-proxy-dev"`。

## 已知问题（不阻塞部署）

- `internal/server/biz/webhook_notifier_test.go:93` — `TestWebhookNotifier_NotifyChannelAutoDisabled` 偶发失败，模板解析返回 `template: webhook:1: unexpected EOF`，与本次品牌改名无关（改名前已存在）。待后续修 webhook 渲染逻辑时同步处理。

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

### 子代理角色分工

- `frontend-worker`：实现前端功能；开始前声明允许/禁止修改的文件。
- `frontend-reviewer`：只做前端改动审查和验证，不与 worker 并发修改同一文件。
- `claude-code`：承担后端或全栈实现；共享契约由单一 owner 维护。
- 每个功能使用独立 worktree；集成 worktree 只做 cherry-pick 合并。子代理完成后必须报告变更文件和验证证据。
