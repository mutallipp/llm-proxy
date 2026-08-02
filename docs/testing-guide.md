# Adapter Gateway 测试规范

用途：给后续 AI 与人类提供按风险分层的验证命令、判断标准和本地 E2E 前置条件。

## 1. 定向 Go 测试

优先运行受影响包，不运行无关的全仓测试：

```bash
go test ./internal/server/orchestrator ./internal/ent/migrate/datamigrate ./internal/server/gql/... -count=1
go test ./internal/server/middleware -run 'TestWithAdapterNoAuthPersistence' -count=1
```

重点覆盖 Adapter selector 的协议隔离、绑定缺失、无可用目标、迁移回填/删除 gate，以及 middleware 的无认证持久化边界。

## 2. 前端验证

类型检查：

```bash
cd frontend && pnpm exec tsc -p tsconfig.app.json --noEmit
```

当前基线约 **175 条既有 TypeScript 错误**；记录基线并修复本次新增错误。协议池与 Adapter 的 Node 回归测试：

```bash
cd frontend
node --test src/features/models/data/protocol-pools.test.mjs
node --test src/features/adapters/adapter-regression.test.mjs
```

## 3. E2E curl 验证

先完成 Channel、Model 协议池和 Adapter 绑定，再使用对应的 source model：

```bash
curl -X POST http://localhost:8090/<adapter>/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"<source_model>","messages":[{"role":"user","content":"ping"}]}'
```

- 不再返回 `no protocol pool` 或 HTTP `422`，说明已通过协议池查找。
- `connection refused` 说明请求已到达 provider 阶段但上游连接不可达；这不是 pool lookup 失败。
- 若出现跨协议命中、`model not bound` 或 `no usable target`，检查 Adapter 入站 format、binding、Model pool key 和 endpoint 前缀。

## 4. E2E 浏览器验证

- UI 测试前必须初始化 owner 账号并登录。
- 本地测试凭据通过仓库根目录 `.env` 配置：

```text
AXONHUB_TEST_EMAIL=...
AXONHUB_TEST_PASSWORD=...
```

- 验证 Model 协议池编辑、GID 回显、Channel endpoint 过滤、Adapter 绑定、刷新后列表和错误状态。

## 5. Docker 重建验证

修改前端或后端后，必须重建并强制重建容器，再硬刷新浏览器：

```bash
docker compose build axonhub
docker compose up -d --force-recreate axonhub
```

确认容器 healthy 后再做 curl/UI 验证；不要把旧容器或旧静态资源的结果当作当前改动结果。

## 6. 范围约束

**除非用户明确授权，不运行全仓 build、lint 或 test。** 定向验证失败、跳过或受环境阻塞时，报告原始错误和未覆盖范围，不静默降级为“通过”。