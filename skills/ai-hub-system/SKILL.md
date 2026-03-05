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
- Session Compress: `POST /api/v1/sessions/:id/compress`（手动压缩会话上下文，body: `{“mode”:”intelligent”}`，mode 可选 `intelligent`/`simple`）
- Chat: `POST /api/v1/chat/send`，`WS /ws/chat`
- Session Rules: `GET/PUT/DELETE /api/v1/session-rules/:id`
- Skills: `GET /api/v1/skills`，`POST /api/v1/skills/toggle`
- MCP: `GET /api/v1/mcp`，`POST /api/v1/mcp/toggle`
- Triggers: `GET/POST/PUT/DELETE /api/v1/triggers`
- Channels: `GET/POST/PUT/DELETE /api/v1/channels`
- Vector: `/api/v1/vector/*`（详见「向量知识库」Skill）
- Status: `GET /api/v1/status`，`GET /api/v1/version`
- Compress Settings: `GET /api/v1/settings/compress`，`PUT /api/v1/settings/compress`（配置自动压缩阈值）
- Export/Import: `GET /api/v1/export/session/:id`，`GET /api/v1/export/team/:name`，`POST /api/v1/import`

说明：团队规则由系统动态注入，不在此 Skill 中维护”项目级规则”流程。

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

## 7. 常见故障案例

### 案例 1：向量引擎启动卡死（管道阻塞）

**现象**：AI Hub 启动或重启向量引擎时，`download_model.py` 进程挂起，端口 8090 无法监听，`vector/status` 返回 `ready: false`。

**根因**：sentence-transformers 加载模型时 tqdm 进度条向 stderr 输出大量数据，Go 父进程管道缓冲区满后未及时读取，Python 子进程在 `write()` 系统调用上阻塞。HuggingFace Hub 在线检查可能因网络问题额外阻塞。

**修复**：在 `download_model.py` 和 `embedding.py` 的 import 前设置环境变量：

```python
os.environ["TQDM_DISABLE"] = "1"
os.environ["HF_HUB_OFFLINE"] = "1"
os.environ["TRANSFORMERS_OFFLINE"] = "1"
```

**注意**：首次使用新模型需手动下载（离线模式无法自动下载）。切换模型时需临时移除 OFFLINE 变量完成下载。

**验证**：重启后 `curl /api/v1/vector/status` 应在 5 秒内返回 `ready: true`。

---

### 案例 2：代理 URL 构造错误（远程系统全面不可用）

**现象**：升级后所有会话在 300-500ms 内返回 `error_during_execution`，完全不可用。本地 OAuth 模式正常。

**根因**：`server/core/claude_pool.go` 的 `spawnProcess()` 将 `session_id` 作为 query parameter 嵌入 `ANTHROPIC_BASE_URL`（`?session_id=XX`）。Anthropic SDK 拼接 `/v1/messages` 时 URL 结构被破坏——path 缺少 `/v1/messages`，query 变成 `session_id=XX/v1/messages`，代理转发到上游根路径。

**修复**：将 session_id 编码到 URL 路径中：

```
旧：http://localhost:PORT/api/v1/proxy/anthropic?session_id=XX
新：http://localhost:PORT/api/v1/proxy/s/SESSION_ID/anthropic
```

路由改为 `/api/v1/proxy/s/:session_id/anthropic/*path`，proxy.go 从路径参数获取 session_id。

**教训**：base URL 中不应包含 query parameter；新增代理功能时必须同时测试 api_key 和 oauth 两种模式。

---

### 案例 3：source_session_id 被 watcher 覆盖为 0

**现象**：团队知识库/记忆库列表的会话ID徽章始终不显示，source_session_id 为 0。

**根因**：`vector_watcher.go` 的文件监视逻辑在检测到文件变化时调用 `syncFileToVector(scope, path, 0)`，覆盖了 API 写入时传入的正确 session_id。

流程：
1. `POST /vector/write` → `SyncFileToVector(scope, path, sessionID=23)` → embed(session_id=23) ✓
2. watcher 检测变化 → `syncFileToVector(scope, path, 0)` → embed 覆盖为 session_id=0 ✗

**修复方向**：watcher 触发 embed 时，先读现有元数据，若 source_session_id > 0 则保留，不覆盖。

**验证**：写入文件后等待 watcher 触发，检查 `list_files` 接口返回的 source_session_id 是否保持非零值。
