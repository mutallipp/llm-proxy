# AxonHub Adapter MVP 部署规范

用途：指导开发者以 Docker Compose 部署单实例 Adapter Gateway，并完成数据库、Adapter 配置与最小链路验证。

## 1. 版本边界与安全

- Adapter 通过 `model_id` 绑定逻辑 Model；Model 的 `protocolPools` 按入站协议保存 Channel、物理模型、优先级和 enabled 配置。
- 管理入口是 `/admin/gateway/*`，Model 协议池在 `/admin/graphql` 编辑；消费入口是 `/{adapter}/v1/...`。
- 这是单实例 MVP，不提供多租户、用户级消费 API Key、配额计费或公网 SaaS 能力。
- **消费入口默认不校验 API Key**：可读取 `Authorization: Bearer`、`x-api-key`、`anthropic-api-key`，但默认放行、不落库、不转发；上游凭证只使用 Channel 自身配置。
- 生产部署必须绑定本机、使用反向代理/WAF 鉴权或开启 IP 白名单，不能直接把 `8090` 暴露到公网。

## Model-centric Adapter Gateway（2026-08-02）

部署和排障都以以下唯一数据流为准：

```text
Adapter source_model_id
  -> AdapterModelBinding.model_id
  -> Model.settings.protocolPools[协议族]
  -> Channel + 物理模型关联
  -> 完整 apiFormat endpoint
  -> provider
```

- 协议池 key 只使用 `openai` 或 `anthropic` 等协议族；`openai/chat_completions`、`anthropic/messages` 等是 Channel endpoint 的完整 `apiFormat`，不能混用。
- `/admin/graphql` 的 Model 编辑器按协议族过滤 Channel endpoint；运行时还会确认启用 Channel 存在匹配完整格式的 endpoint，不做跨协议兜底或隐式转换。
- Adapter binding 只负责 alias 到逻辑 Model。协议池、Channel endpoint 或物理模型变更后，保存并刷新 snapshot 即可生效，不需要修改 binding 或重启。
- 旧 ModelGroup 仅由 `datamigrate beta7` 迁移到 Model 协议池；迁移成功并通过 drop gate 后，运行时不再读取旧 ModelGroup。

## 2. 数据库迁移与兼容边界

- `datamigrate beta7` 自动执行 `ModelGroup → Model.protocolPools` 回填，并回填 Adapter 的 `model_id` binding。
- drop gate 在回填、校验成功后自动删除旧三表及其约束/索引；迁移失败按设计 **fail-fast**，不启动新运行时，不产生半成品。
- MySQL legacy 数据库不作为自动兼容路径：按设计 fail-fast，必须先手动迁移或清空旧数据。
- 已验证的部署路径是 PostgreSQL（Compose 默认）；SQLite 仅用于开发，不用于生产。
- 升级前备份数据库，并确认现有 Adapter binding 已使用 `model_id`。

## 3. 部署顺序

前置条件：Docker Engine ≥ 24、Docker Compose v2、端口 `8090` 可用；当前目录为包含 `Dockerfile` 和 `docker-compose.yml` 的仓库根目录。

```bash
# 1. 构建镜像
docker compose build axonhub
# 2. 确保 PostgreSQL 可达后启动
docker compose up -d
# 3. 等待健康状态
docker compose ps
docker compose logs -f axonhub | head -50
```

确认 `axonhub` 显示 `healthy` 后，再初始化或登录浏览器；不要在容器尚未 healthy 时开始 UI 验证。

```bash
curl -fsS http://localhost:8090/health
```

首次启动会生成 owner 初始化 token，可从日志提取：

```bash
docker compose logs axonhub | grep -i 'initialize\|owner\|one-time'
curl -X POST http://localhost:8090/admin/system/initialize \
  -H 'Content-Type: application/json' \
  -d '{"ownerEmail":"you@example.com","ownerPassword":"<强密码>","ownerName":"owner"}'
```

随后在浏览器登录，或调用 `/admin/auth/signin` 获取 owner JWT；管理面请求需要该 JWT。

## 4. Adapter 配置流程

严格按以下顺序配置，刷新快照后再消费：

1. **创建 Channel**：启用 Channel，填写上游凭证，并配置支持目标协议的 endpoint。
2. **创建 Model**：在 `/admin/graphql` 的 Model settings 中配置 `protocolPools`。协议池 key 使用 `openai` 或 `anthropic`，关联填写 `channel_model`、物理模型、priority 和 enabled；不填写 outbound 字段。
3. **创建 Adapter**：在 `/admin/gateway/adapters` 绑定 `source_model_id → model_id`，不绑定 Channel 或物理模型。
4. **刷新快照**：保存后调用刷新接口，确保请求读到新配置。
5. **curl 验证**：用对应 Adapter path 和 source model 发起请求，确认没有 `no protocol pool`/`422`。

示例（数字 `model_id` 应取实际 Model ID）：

```bash
TOKEN=...
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  http://localhost:8090/admin/gateway/adapters/pi-anthropic \
  -d '{"name":"pi-anthropic","displayName":"Pi via Anthropic",\
"inboundAPIFormat":"anthropic_messages","status":"enabled",\
"bindings":[{"source_model_id":"fast","model_id":1,"enabled":true}]}'

curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8090/admin/gateway/refresh

curl -X POST http://localhost:8090/pi-anthropic/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"model":"fast","messages":[{"role":"user","content":"ping"}]}'
```

Adapter binding 只负责 alias 到逻辑 Model；真实目标的切换在协议池中完成，保存并刷新后无需修改 binding 或重启。

## 5. 反向代理与宿主机代理

生产推荐使用：

```text
deploy/nginx-adapter-mvp.conf
```

反向代理应截断旧 `/v1`、`/anthropic/v1`、`/gemini/*`、`/oauth/*` 入口，并对 Adapter path 增加外部鉴权。修改 Nginx 后检查并 reload：

```bash
sudo nginx -t && sudo systemctl reload nginx
```

Compose 已配置 `host.docker.internal:host-gateway`。需要宿主机代理时，在不提交的 `.env` 中设置：

```text
AXONHUB_HTTP_PROXY=http://host.docker.internal:<proxy-port>
AXONHUB_HTTPS_PROXY=http://host.docker.internal:<proxy-port>
AXONHUB_ALL_PROXY=http://host.docker.internal:<proxy-port>
AXONHUB_NO_PROXY=localhost,127.0.0.1,postgres,redis,host.docker.internal
```

代理程序必须监听 Docker 网关可达的接口；不要把个人代理地址写入 Compose 或提交。

## 6. 故障排查、升级与回滚

| 现象 | 检查 |
|---|---|
| `401` on Adapter path | 确认命中 `/:adapter` 动态路由；MVP consumer no-auth 配置未被覆盖。 |
| `api.auth.allow_no_auth` 警告 | 确认 Compose 中 `AXONHUB_SERVER_API_AUTH_ALLOW_NO_AUTH: "true"` 与预期一致。 |
| 管理接口 `401` | 使用 owner JWT 登录 `/admin/auth/signin`。 |
| Postgres 起不来 | 查看 `docker compose logs postgres`，核对密码和可达性。 |
| Adapter 修改不生效 | 调 `POST /admin/gateway/refresh`，并确认容器/快照版本。 |
| `no protocol pool` | 检查入站协议族 key、Model settings 和 binding。 |

保留数据升级：

```bash
docker compose pull
docker compose up -d
```

回滚到备份分支时先停止服务，再切换代码并重建；不要在运行中的容器上混用旧镜像和新数据库：

```bash
docker compose down
git checkout <备份分支或版本>
docker compose up -d --build
```

已知限制：旧通用入口仍可能监听，需由反代/防火墙挡掉；消费 API Key 尚未校验；没有多目标压测、真实客户端响应 fixture、Vault/HSM 集成和强制改密流程。