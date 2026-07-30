# AxonHub Adapter MVP 部署文档

> 单实例适配器网关（MVP）的本地 Docker 部署指南。
> 目标使用者：希望把 Pi / Claude Code / Codex 等客户端指到一个稳定的逻辑模型网关，并能在后台切换真实渠道模型的开发者。

## 1. 这个版本是什么

- **是什么**：在 AxonHub 上做了一次裁剪式二开。新增 5 个 Ent 表（`Adapter`、`AdapterModelBinding`、`ModelGroup`、`ModelGroupProtocol`、`ModelGroupTarget`）和一个管理入口（`/admin/gateway/*`）。消费端通过 `/{adapterName}/v1/...` 接入，模型组负责把逻辑模型映射到真实渠道目标。
- **不是什么**：不是多租户 / 多用户平台，不是带用户级 API Key 的代理，不是面向公网的 SaaS 网关。
- **MVP 边界**：消费入口默认不校验 API Key；后台 `/admin/gateway/*` 复用现有 owner 登录；旧 `/v1`、`/anthropic/v1` 等通用入口仍然监听（详见 §3）。

## 2. 安全边界（部署前必读）

⚠️ **本版本消费入口不校验 API Key。** `WithAdapterConsumerInterceptor` 只读取消费层传入的 Key（`Authorization: Bearer`、`x-api-key`、`anthropic api-key`），但默认放行、不落库、不转发。这意味着：

- **任何能访问到端口的人都能发起消费请求**。
- 上游请求始终使用 Channel 自己的凭证，不会泄露消费层 Key。

部署时必须用以下任意一种方式隔离：

1. **本机使用**：只绑定到 `127.0.0.1`，不暴露端口。
2. **反代隔离**：使用 [`deploy/nginx-adapter-mvp.conf`](../../deploy/nginx-adapter-mvp.conf) 截断 `/v1`、`/anthropic/v1`、`/gemini/*`、`/oauth/*` 等旧入口（§6）。
3. **IP 白名单**：开启 `server.ip_access_control.enabled: true`，只放行可信网段。
4. **公网出口**：不要直接放公网；若必须放，前置 WAF + 鉴权代理是前提。

未来会提供全局 API Key 和 Adapter 级 API Key 校验开关；详见 [plan 2026-07-29-001](../plans/2026-07-29-001-feat-single-instance-adapter-gateway-plan.md)。

## 3. 默认行为速查

| 路径 | 鉴权 | MVP 是否启用 |
|---|---|---|
| `/{adapter}/v1/chat/completions`、`/{adapter}/v1/responses`、`/{adapter}/v1/messages`、`/{adapter}/v1/models` | 消费入口 | ✅ 已挂载，**默认放行** |
| `/admin/gateway/adapters` 等 6 个 | 现有 owner JWT | ✅ 已挂载 |
| `/admin/...` 其余（频道、API Key、项目、用户） | 现有 owner JWT | ✅ 保留 |
| `/health` | 无 | ✅ |
| `/v1`、`/anthropic/v1`、`/gemini/*` 等旧通用入口 | 旧 APIKey 体系 | ⚠️ **仍监听**，用反代或防火墙挡掉 |
| `/oauth/*`、`/admin/auth/signin` | 旧登录 | ⚠️ 仍监听 |

## 4. 启动（Docker Compose）

### 4.1 准备工作

- 已装 Docker Engine ≥ 24，Docker Compose v2。
- 本机端口 `8090` 未被占用。
- 当前工作目录就是仓库根（含 `Dockerfile`、`docker-compose.yml`）。

### 4.2 启动

```bash
docker compose up -d --build
docker compose logs -f axonhub | head -50
```

第一次构建会比较久（Go + 前端两阶段）。等待 axonhub 容器显示 `healthy`：

```bash
docker compose ps
```

预期：

```text
NAME                STATUS              PORTS
axonhub-postgres    healthy (starting)   5432/tcp
axonhub-app         healthy              0.0.0.0:8090->8090/tcp
```

### 4.3 初始化 owner

容器首次启动会自动调用 `/admin/system/initialize`。日志里会打印 owner 邮箱和一次性初始化 token（**生产部署必须从日志抓取**）：

```bash
docker compose logs axonhub | grep -i 'initialize\|owner\|one-time'
```

按日志里的 token 调初始化接口：

```bash
curl -X POST http://localhost:8090/admin/system/initialize \
  -H 'Content-Type: application/json' \
  -d '{
    "ownerEmail": "you@example.com",
    "ownerPassword": "<强密码>",
    "ownerName": "owner"
  }'
```

成功后用 `/admin/auth/signin` 登录拿到 JWT（用于后续管理 API）。

### 4.4 验证最小链路

```bash
# 1. 健康检查
curl -fsS http://localhost:8090/health

# 2. 管理接口（带 JWT）
TOKEN=...
curl -fsS -H "Authorization: Bearer $TOKEN" http://localhost:8090/admin/gateway/runtime
```

## 5. 第一个 Adapter（最小可用配置）

下面走一遍从零创建 `pi-anthropic` 的流程，让 Pi 客户端可以指过来。

### 5.1 创建一个 Channel

Channel 是真实上游，凭证放这里。**MVP 不动原 Channel 管理入口**，所以直接用 `/admin/channels` 的现有 API（或后台页面）。

### 5.2 创建 ModelGroup + ModelGroupProtocol

```bash
TOKEN=...

# 1. 创建 ModelGroup
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  http://localhost:8090/admin/gateway/model-groups \
  -d '{
    "name": "opus-default",
    "displayName": "Opus Default",
    "selectionStrategy": "priority_failover"
  }'

# 2. 给 ModelGroup 加一个入站协议（Anthropic）
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  http://localhost:8090/admin/gateway/model-groups/opus-default \
  -d '{
    "name": "opus-default",
    "displayName": "Opus Default",
    "selectionStrategy": "priority_failover",
    "protocols": [
      { "inboundAPIFormat": "anthropic_messages", "enabled": true }
    ],
    "targets": [
      {
        "channelId": 1,
        "targetModelId": "claude-opus-4-20250514",
        "outboundAPIFormat": "anthropic_messages",
        "priority": 10,
        "enabled": true
      }
    ]
  }'
```

> targets 是按入站协议拆分的；同一个 ModelGroup 想同时服务 Anthropic 和 OpenAI 入口，就要分别给 `anthropic_messages` 和 `openai_responses` 配置自己的 targets。

### 5.3 创建 Adapter + 绑定

```bash
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  http://localhost:8090/admin/gateway/adapters/pi-anthropic \
  -d '{
    "name": "pi-anthropic",
    "displayName": "Pi via Anthropic",
    "inboundAPIFormat": "anthropic_messages",
    "status": "enabled",
    "bindings": [
      { "sourceModelId": "fast",   "modelGroupName": "opus-default", "enabled": true },
      { "sourceModelId": "opus",   "modelGroupName": "opus-default", "enabled": true }
    ]
  }'
```

### 5.4 Pi 客户端指向网关

```bash
export ANTHROPIC_BASE_URL="http://localhost:8090/pi-anthropic"
export ANTHROPIC_AUTH_TOKEN="not-used"   # 一期不校验
```

Pi 客户端用 `model: fast` 或 `model: opus` 请求，会自动落到 `claude-opus-4-20250514`。

### 5.5 切换真实目标

```bash
# 把 pi-anthropic 的"opus"绑到另一个 ModelGroup，或者改 targets 里的 targetModelId
# 保存后立即生效，无需重启
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8090/admin/gateway/refresh
```

## 6. 反向代理（生产部署）

不要直接暴露 `8090`。使用 [`deploy/nginx-adapter-mvp.conf`](../../deploy/nginx-adapter-mvp.conf)：

```nginx
http {
    upstream axonhub_backend {
        server 127.0.0.1:8090;
        keepalive 32;
    }

    server {
        listen 443 ssl http2;
        server_name llm.example.com;

        ssl_certificate     /etc/nginx/ssl/cert.pem;
        ssl_certificate_key /etc/nginx/ssl/key.pem;

        # 把 adapter-mvp.conf 里的三个 location 块复制进来
        include /etc/nginx/snippets/axonhub-adapter-mvp.conf;
    }
}
```

生效：

```bash
sudo nginx -t && sudo systemctl reload nginx
```

## 7. 故障排查

| 现象 | 检查 |
|---|---|
| 启动报 `api.auth.allow_no_auth` 警告 | 确认 `docker-compose.yml` 环境变量 `AXONHUB_SERVER_API_AUTH_ALLOW_NO_AUTH: "true"` |
| `401 unauthorized` on `/{adapter}/v1/...` | 旧入口在收请求，确认请求路径确实命中 `:adapter` 动态路由；或先关 `allow_no_auth` 看错误码变化 |
| `/admin/gateway/adapters` 返回 401 | 需要 owner JWT；先用 `/admin/auth/signin` 登录 |
| Postgres 起不来 | 看 `docker compose logs postgres`；通常是密码不匹配 |
| 修改 Adapter 不生效 | 调 `POST /admin/gateway/refresh` 手动刷新，或等下一次请求自动 Refresh |

## 8. 回滚到 AxonHub 上游

如果需要回到二开前的 AxonHub：

```bash
# 在原始克隆仓库（非 worktree）
git fetch origin
git checkout backup-pre-axonhub-mvp    # fork 上轮创建的备份分支
docker compose down
git checkout main
docker compose up -d --build
```

或者从 GitHub 切到 fork 之前的镜像版本。

## 9. 升级流程（保留数据）

```bash
docker compose pull        # 拉新镜像（或 docker compose build --pull）
docker compose up -d       # 滚动重启
docker compose exec axonhub axonhub config migrate  # 如果有手动迁移步骤
```

MVP 阶段没有独立 migration 命令，Ent 自动迁移在启动时跑。

## 11. 公司 VPN / 宿主机代理配置

`docker-compose.yml` 已内置 `extra_hosts: host.docker.internal:host-gateway`，容器可通过 `host.docker.internal` 访问宿主机网络服务（如宿主机代理程序）。

代理地址 **不硬编入 docker-compose.yml**，而是通过环境变量按需注入。

### 11.1 本地开发：通过 `.env` 文件注入

> `.env` 已在 `.gitignore` 中，**不要将它提交到 Git**。

在仓库根目录创建 `.env`（不提交）：

```bash
# .env 示例——占位符请替换为宿主机实际代理地址和端口

# 使用项目命名空间变量 AXONHUB_*，避免宿主机环境中同名的 HTTP_PROXY 等变量泄入容器。
# 宿主机必须允许来自 Docker 网关（默认 172.17.0.1）的连接
AXONHUB_HTTP_PROXY=http://host.docker.internal:<proxy-port>
AXONHUB_HTTPS_PROXY=http://host.docker.internal:<proxy-port>
AXONHUB_ALL_PROXY=http://host.docker.internal:<proxy-port>
# 内部地址跳过代理
AXONHUB_NO_PROXY=localhost,127.0.0.1,postgres,redis,host.docker.internal
```

完成后重启服务使配置生效：

```bash
docker compose up -d
```

### 11.2 CI / OpenStack / 云环境：通过平台环境变量注入

在 CI 系统或 OpenStack 渲染器配置中设置以下环境变量（占位符请替换为实际地址）：

```
AXONHUB_HTTP_PROXY=http://<proxy-host>:<proxy-port>
AXONHUB_HTTPS_PROXY=http://<proxy-host>:<proxy-port>
AXONHUB_ALL_PROXY=http://<proxy-host>:<proxy-port>
AXONHUB_NO_PROXY=localhost,127.0.0.1,postgres,redis,host.docker.internal
```

### 11.3 没有代理（普通部署）

无需任何操作。`AXONHUB_HTTP_PROXY` 等变量默认为空字符串，容器内 `HTTP_PROXY` 也为空，不影响正常网络请求。

### 11.4 宿主机代理注意事项

- 宿主机代理程序必须监听全部网络接口（不能只绑 `127.0.0.1`），才能接收来自 Docker 网关的连接。
- 宿主机代理的 Socks5/HTTP 地址和端口属个人 / 公司配置，**不将实际地址写入任何配置文件或 commit**。
- macOS 下 `host-gateway` 在 Docker Desktop 4.x 及以上已直接支持，无需额外设置。
- Linux 宿主机：`host-gateway` 映射到 Docker bridge 网关 IP（默认 `172.17.0.1`），确保防火墙放行该 IP 的入站流量到代理端口。

## 10. 已知缺口（不要被吹过头）

- ❌ 没有针对负向用例（坏协议、坏 key、超时）的 e2e 测试
- ❌ 没有真实多目标故障转移压测
- ❌ 没有针对 Pi/Claude Code/Codex 的真实响应侧 fixture（Phase 0 还没完成）
- ❌ 旧 `/v1` 等入口**没有开关**，需要反代挡掉
- ❌ 消费入口不校验 API Key（这是 MVP 设计）
- ⚠️ Admin owner 密码第一次初始化后没有强制改密流程
- ⚠️ 没有 HSM、Vault 等密钥管理集成

完整范围边界见 [plan 2026-07-29-001](../plans/2026-07-29-001-feat-single-instance-adapter-gateway-plan.md)。