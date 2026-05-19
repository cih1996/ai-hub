---
name: "一号核心手册"
description: "系统核心操作手册入口。当需要操作记忆库、知识库、会话、规则、笔记、定时器、钩子、服务、技能、Schema、挂载、传输、系统诊断、脚本引擎时触发。AI 应先 Read 本手册，再按需继续 Read CLI.md 或 HTTP.md。"
when_to_use: "当需要操作记忆库、知识库、会话、规则、笔记、定时器、钩子、服务、技能、Schema、挂载、传输、系统诊断、脚本引擎时"
---

# 一号核心手册

> Skill 名称：一号核心手册（ai-hub-core）
> 触发条件：当需要操作记忆库、知识库、会话、规则、笔记、定时器、钩子、服务、技能、Schema、挂载、传输、系统诊断、脚本引擎时触发。
> 文档结构：主手册 + `CLI.md` + `HTTP.md`
> 工具数量：文件系统工具 + CLI 命令 + HTTP API

---

## §1 阅读顺序

- 本文件只保留执行原则、数据分层和文档路由，不再堆叠所有命令/API 细节。
- 需要走命令行时，继续 `Read` 同目录下的 `CLI.md`。
- 需要走 HTTP 接口时，继续 `Read` 同目录下的 `HTTP.md`。
- 同时存在 CLI 与 HTTP 时，优先判断当前任务是否更适合：
  - 本地文件直读直写：优先文件系统工具
  - 本地服务已运行且接口稳定：优先 HTTP
  - 用户明确要求命令行、批处理或跨机调用：优先 CLI
- `services` 已纳入本技能，CLI 与 HTTP 都已记录在子文档中。

## §2 执行原则

### 2.1 Skill 优先

- Skill 是执行协议，不是参考文档。
- 记忆库、知识库、团队/全局规则优先通过文件系统工具直接操作。
- 需要脱离本地文件系统访问时，使用 HTTP API。
- 发现“执行不流畅、重复步骤、规则不足”时，优先补充或修订 Skill。

### 2.2 CLI 与 HTTP 的关系

- 大多数 CLI 命令只是 `/api/v1` 的薄封装。
- 少数 CLI 会直接操作本地文件系统或系统服务：
  - `rules --level global|team`：直接读写 `~/.ai-hub` 下的规则文件
  - `daemon install/uninstall/start/stop`：主要调用 launchd/systemd/windows service
  - `transfer`：直接请求目标 AI Hub 的 HTTP 接口
- 文档如有冲突，以源码中的真实注册路由为准：`main.go` > 接口注释 > 历史文档。

### 2.3 环境变量

CLI 命令自动继承以下环境变量（由 AI Hub 进程注入）：

| 层级 | 作用域 | 说明 |
|------|------|------|
| `AI_HUB_SESSION_ID` | 当前会话 ID | 多数会话相关 CLI 的默认上下文 |
| `AI_HUB_GROUP_NAME` | 当前团队名 | 团队级文件、团队规则、团队会话默认上下文 |
| `AI_HUB_PORT` | 服务端口 | 默认 `9527` |

也可通过全局 flag 覆盖：`--session <id>` / `--group <name>` / `--port <port>`

### 2.4 三层数据隔离

所有记忆类数据遵循三层隔离：

| 层级 | 作用域 | 说明 |
|------|--------|------|
| 会话级 | `<group>/sessions/<id>/memory` | 当前会话私有 |
| 团队级 | `<group>/memory` | 同团队共享 |
| 全局级 | `memory` | 所有会话可见 |

搜索时自动合并三层结果，优先级：会话 > 团队 > 全局。

### 2.5 记录治理

- 先搜索后写入，命中则更新，避免重复。
- 每个主题一个主文件，禁止按日期命名。
- 正文写当前状态，变更追加到「变更记录」章节。
- 禁止在正文写过程叙述。

### 2.6 诊断优先

- 遇到问题先诊断再修复，禁止盲目操作。
- 优先用 API、CLI 查询或文件工具读取当前状态。
- 安全重启遵循：`kill/stop -> wait -> verify`。
- 日志优先查看：
  - `~/.ai-hub/logs/ai-hub.log`
  - `[proxy]` 行用于排查模型/代理请求
  - `[hooks]` 行用于排查事件钩子

### 2.7 调度安全

- 执行类调度必须带上下文头：`[group_name|scope|target|task_id]`
- 子会话回调必须带同一 `task_id`
- 未锁定 `scope/target` 的执行任务不得下发

## §3 常用路径

### 3.1 记忆库

| 层级 | 路径 |
|------|------|
| 全局 | `~/.ai-hub/memory/*.md` |
| 团队 | `~/.ai-hub/teams/<group>/memory/*.md` |
| 会话 | `~/.ai-hub/teams/<group>/sessions/<id>/memory/*.md` |

### 3.2 知识库

- 根目录：`~/.ai-hub/knowledge/`
- 按项目或主题组织子目录，例如：`project-a/api-spec.md`

### 3.3 规则

| 层级 | 路径 | 说明 |
|------|------|------|
| 全局 | `~/.ai-hub/rules/CLAUDE.md` | 模板文件，支持 `{{VAR}}` |
| 团队 | `~/.ai-hub/teams/<group>/rules/*.md` | 团队私有 |
| 会话 | `~/.ai-hub/session-rules/{id}.md` | 会话级角色定义，优先级最高 |

### 3.4 笔记与脚本

- 笔记：`~/.ai-hub/notes/`
- 脚本仓库：`~/.ai-hub/scripts/`
- 脚本索引：`~/.ai-hub/scripts/INDEX.md`

## §4 子文档索引

### `CLI.md`

适用于以下场景：

- 需要查命令名、flag、子命令和实操示例
- 需要确认某个 CLI 是否只是 HTTP 封装
- 需要使用 `services`、`daemon`、`reload`、`transfer`、`skills`、`schemas`、`errors`、`changelog` 等旧手册未覆盖能力

### `HTTP.md`

适用于以下场景：

- 需要直接调用 REST API
- 需要把 CLI 行为映射到真实接口
- 需要确认某条接口是否真实注册
- 需要补齐 `services`、`hooks`、`skills`、`schemas`、`transfer`、`mounts`、`reload`、`shutdown` 等接口

## §5 脚本化规范

- 多步重复操作（>= 3 步）必须脚本化，禁止逐步单条交互。
- 先查 `INDEX.md` 是否有可复用脚本；没有再新建。
- Shell 脚本必须 `set -euo pipefail`。
- 禁止硬编码 URL、端口、会话 ID、团队名，全部参数化。
- 失败时优先修复脚本并重跑，禁止回退到手工逐步操作。
