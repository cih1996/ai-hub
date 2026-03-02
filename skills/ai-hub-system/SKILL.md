---
name: "一号系统感知"
description: "AI Hub 系统运维与诊断 Skill。用于版本检查、日志排障、会话/向量/认证/代理问题定位与安全修复。"
---

# 系统自感知 Skill（v2）

你运行在 AI Hub 内，不是独立 CLI。目标是：快速定位问题、给出可验证结论、最小化风险操作。

## 0. 执行原则

1. 先诊断后修复：先收集证据，再执行变更。
2. 先结论后证据：先告诉用户“原因/状态”，再附关键命令输出摘要。
3. 最小破坏：默认不用 `kill -9`、不自动安装系统依赖、不覆盖用户配置。
4. Skill-first：系统管理动作优先通过 AI Hub API 完成。

## 1. 基础信息

- Base URL: `http://localhost:$AI_HUB_PORT/api/v1`
- Web: `http://localhost:$AI_HUB_PORT`
- 日志: `~/.ai-hub/logs/ai-hub.log`
- 数据目录: `~/.ai-hub/`

快速检查：

```bash
curl -s http://localhost:$AI_HUB_PORT/api/v1/version
curl -s http://localhost:$AI_HUB_PORT/api/v1/status
```

## 2. 常用诊断入口

### 2.1 服务与进程

```bash
lsof -i:$AI_HUB_PORT
curl -s http://localhost:$AI_HUB_PORT/api/v1/sessions
```

判定：端口被占用或服务无响应时，先确认占用进程，再决定是否重启。

### 2.2 日志定位

```bash
tail -80 ~/.ai-hub/logs/ai-hub.log
grep -Ei "error|forbidden|403|timeout|vector|auth|proxy" ~/.ai-hub/logs/ai-hub.log | tail -50
```

判定：优先提取最近一次失败请求对应的错误链路。

### 2.3 向量引擎

```bash
curl -s http://localhost:$AI_HUB_PORT/api/v1/vector/status
curl -s http://localhost:$AI_HUB_PORT/api/v1/vector/stats
```

异常时可重启一次：

```bash
curl -s -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/restart
```

### 2.4 认证/403/代理问题（高频）

检查顺序：
1. Provider 配置是否命中当前会话。
2. 环境变量是否被全局配置覆盖（如 `~/.claude/settings.json`、`HTTP_PROXY/HTTPS_PROXY`）。
3. 上游是否返回限流/封禁（`http_ratelimit`、`forbidden`）。
4. 是否走了错误 base_url 或空 scheme（例如 `unsupported protocol scheme ""`）。

建议命令：

```bash
curl -s http://localhost:$AI_HUB_PORT/api/v1/providers
curl -s http://localhost:$AI_HUB_PORT/api/v1/sessions
```

## 3. 核心 API（精简）

- Providers: `GET/POST/PUT/DELETE /api/v1/providers`
- Sessions: `GET /api/v1/sessions`，`GET /api/v1/sessions/:id`，`DELETE /api/v1/sessions/:id`
- Chat: `POST /api/v1/chat/send`，`WS /ws/chat`
- Session Rules: `GET/PUT/DELETE /api/v1/session-rules/:id`
- Skills: `GET /api/v1/skills`，`POST /api/v1/skills/toggle`
- MCP: `GET /api/v1/mcp`，`POST /api/v1/mcp/toggle`
- Triggers: `GET/POST/PUT/DELETE /api/v1/triggers`
- Vector: `/api/v1/vector/*`
- Status: `GET /api/v1/status`，`GET /api/v1/version`

说明：团队规则由系统动态注入，不在此 Skill 中维护“项目级规则”流程。

## 4. 安全重启流程（推荐）

1. 记录当前状态：版本、端口占用、最近日志。
2. 温和停止：先 `kill <pid>`，仅在确认失控时再升级信号。
3. 启动后验证：`/api/v1/version`、`/api/v1/status`、发一条最小测试消息。
4. 输出结果：重启前后差异 + 验证结论。

## 5. 系统自检模板

触发词：初始化系统、系统自检、环境检查。

最小自检项：
1. 服务可达（version/status）。
2. 会话系统可达（sessions）。
3. 向量状态可达（vector/status）。
4. 日志无持续性致命错误（最近 80 行）。

输出格式：

```text
AI Hub 自检结果
- 服务状态：正常/异常（证据）
- 会话系统：正常/异常（证据）
- 向量系统：正常/异常（证据）
- 关键告警：<若有>
- 建议动作：<最多3条>
```

## 6. Skill 演进要求

出现以下任一情况时，更新本 Skill：
- 同类故障连续出现 2 次以上。
- 诊断步骤顺序导致误判或漏判。
- API 行为发生变化。

更新记录建议包含：问题、根因、改动点、适用边界、验证方式。
