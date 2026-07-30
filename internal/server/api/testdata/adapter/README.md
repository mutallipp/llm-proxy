# 适配器入站协议样例

这些文件是 2026-07-29 在本机通过临时 loopback HTTP 捕获服务采集的真实客户端入站请求，用于设计 `/:adapter/v1` 路由和协议适配器。采集时只使用固定提示词与假的本地 token，没有访问真实模型上游。

## 样例清单

| 文件 | 客户端/版本 | 入站路径 | 观测结果 |
| --- | --- | --- | --- |
| `pi-responses.request.json` | Pi；请求头中的 OpenAI JS `6.26.0` | `POST /pi/v1/responses` | `stream=true`，包含 system/user input，基础请求无 tools |
| `pi-responses-tool.request.json` | Pi | `POST /pi/v1/responses` | `stream=true`，包含 1 个 `read` 工具定义 |
| `claude-code-messages.request.json` | Claude CLI `2.1.119`；Anthropic SDK `0.81.0` | `POST /v1/messages?beta=true` | `stream=true`，Anthropic Messages 请求，含 Claude Code system blocks |
| `claude-code-messages-tool.request.json` | Claude CLI `2.1.119`；Anthropic SDK `0.81.0` | `POST /v1/messages?beta=true` | `stream=true`，包含 1 个 `Bash` 工具定义 |
| `codex-responses.request.json` | Codex `0.132.0` | `POST /v1/responses` | `stream=true`，包含 11 个内置工具定义和 Codex 元数据 |

对应的 `*.headers.json` 保存了实际观测到的非敏感请求头；认证值、会话 ID、请求 ID、安装 ID 和主机名已替换为占位符。

## 已确认的路由和头部差异

- Pi 和 Codex 都使用 OpenAI Responses 请求体，但 Pi 的请求入口应独立于 Codex；不能只按 `/v1/responses` 判断适配器。
- Claude Code 使用 Anthropic Messages 请求体，并带 `anthropic-version`、`anthropic-beta`、`anthropic-dangerous-direct-browser-access`、`x-app` 和 `X-Claude-Code-Session-Id` 等头部。
- Codex 使用 Responses 请求体，并带 `Session-Id`、`Thread-Id`、`Originator`、`X-Codex-Turn-Metadata`、`X-Codex-Window-Id` 和 `X-Codex-Beta-Features` 等头部。
- Pi 的本次非交互捕获中 HTTP `Accept` 为 `application/json`，但请求体 `stream=true`；Codex 的 `Accept` 为 `text/event-stream`。适配器不能只依赖 `Accept` 推断流式模式，应以协议请求体和客户端行为为准。

## 采集边界

已采集：真实客户端请求路径、请求头、非流式语义字段、流式请求标志和工具定义入口。

尚未采集：由真实上游返回并被三个客户端完整消费的 SSE 事件序列、真实工具调用回合、真实上游错误响应及客户端对错误的最终展示。仓库现有 `llm/transformer/*/testdata` 中的响应流只能作为转换器回归数据，不能标记为客户端抓包。

因此在实现适配器前仍需补充三类脱敏样例：

1. 文本流：请求成功并完整结束；
2. 工具流：模型发起工具调用，客户端发出下一回合工具结果；
3. 错误流：限流、鉴权失败、超时/断流和协议错误各至少一例。
