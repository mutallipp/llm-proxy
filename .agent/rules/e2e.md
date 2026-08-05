---
alwaysApply: false
globs: "frontend/tests/**/*.ts, scripts/e2e/**/*.sh"
---

# E2E 测试规则

1. 为稳定定位元素添加 `data-testid`。
2. 本地前端登录优先从环境变量读取账号，不把真实凭据写入代码、文档、日志或提交。现有 Playwright 配置使用 `LLM_PROXY_ADMIN_EMAIL`、`LLM_PROXY_ADMIN_PASSWORD`；未配置时才使用测试夹具约定的本地默认值 `my@example.com` / `pwd123456`。

## 变更必跑 E2E（2026-08-06）

1. 任何新增功能或修改既有功能的提交，必须在 `frontend/tests/` 补上对应的 E2E 用例（创建/编辑/删除/状态切换/错误状态等关键路径），与单元/集成测试同步进入同一改动。未经 E2E 覆盖的功能不得宣称完成。
2. 运行命令统一使用仓库内置脚本，不调用项目外的 `pnpm exec`/`npm exec`：

```bash
./scripts/e2e/e2e-test.sh --project=chromium <spec file>
```

3. 全局 Playwright 与浏览器由本机全局安装提供（`playwright --version` 应为 `1.58.2`）；不下载项目本地浏览器副本。脚本启动 Vite dev server（端口 9527）并清理后端，无需额外手动启动服务。
4. E2E 中只允许使用项目白名单凭据（环境变量优先、夹具默认值兜底），不允许把真实凭据、测试账号密码硬编码到 spec、组件或 fixtures。

## Adapter Gateway 定向验证（2026-08-02）

### 定向测试命令

只运行与改动匹配的验证，不以未执行的命令代替证据：

```bash
go test ./internal/server/orchestrator ./internal/ent/migrate/datamigrate ./internal/server/gql/... -count=1
go test ./internal/server/middleware -run 'TestWithAdapterNoAuthPersistence' -count=1

cd frontend
pnpm exec tsc -p tsconfig.app.json --noEmit
node --test src/features/models/data/protocol-pools.test.mjs
node --test src/features/adapters/adapter-regression.test.mjs
```

前端类型检查当前约有 175 条既有 TypeScript 错误；验证时记录基线，只修复本次新增错误，不得把新增错误归入基线。

### E2E 前置与 curl 验证

1. 先启用 Channel，并为目标协议配置 endpoint；再在 `/admin/graphql` 为 Model 配置对应协议池，最后在 `/admin/gateway/adapters` 建立 `source_model_id -> model_id` binding。
2. 保存后调用 `POST /admin/gateway/refresh`，确认 runtime snapshot 已更新，再调用 Adapter path：

```bash
curl -X POST http://localhost:8090/<adapter>/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"<source_model>","messages":[{"role":"user","content":"ping"}]}'
```

3. `no protocol pool`、`model not bound`、`no usable target` 或 HTTP `422` 时，检查 Adapter 入站格式、binding、Model pool key 和 endpoint；`connection refused` 表示已进入 provider 连接阶段，不等同于协议池查找失败。
4. 负向验证必须确认 OpenAI Adapter 不命中 Anthropic pool，反之亦然。

### 浏览器 E2E 与本地凭据

- 测试前必须初始化 owner 并登录；Adapter Gateway 本地测试凭据通过仓库根目录 `.env` 提供：

```text
LLM_PROXY_TEST_EMAIL=...
LLM_PROXY_TEST_PASSWORD=...
```

- 不提交 `.env` 中的真实值。浏览器至少验证 Model 协议池编辑、Channel endpoint 过滤、GID 回显、Adapter binding、刷新后的列表和错误状态。

### Docker 静态资源验收

修改前端或后端后，需按环境需要重建并强制重建容器，再硬刷新浏览器：

```bash
docker compose build llm-proxy
docker compose up -d --force-recreate llm-proxy
```

确认容器 healthy 后再做 curl/UI 验证；不要把旧容器或旧静态资源的结果当作当前改动结果。

## 范围约束

除非用户明确授权，不运行全仓 build、lint 或 test，也不重启开发服务器。定向验证失败、跳过或受环境阻塞时，报告原始错误和未覆盖范围，不静默降级为“通过”。
