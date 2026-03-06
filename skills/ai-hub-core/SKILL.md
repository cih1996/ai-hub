---
name: "一号核心手册"
description: "系统核心操作手册。当需要操作记忆库、会话、规则、笔记、定时器、系统诊断、脚本引擎时触发。CLI 用法已内置在全局规则中，无需 Read 本手册。"
---

# 一号核心手册

> Skill 名称：一号核心手册（ai-hub-core）
> 触发条件：当需要操作记忆库、会话、规则、笔记、定时器、系统诊断、脚本引擎时触发。
> 工具数量：CLI 命令 + HTTP API

---

## §1 执行原则

### 1.1 Skill 优先

- Skill 是执行协议，不是参考文档。
- 能走 CLI 的操作必须走 CLI，禁止手动 curl 拼接。
- 发现"执行不流畅、重复步骤、规则不足"时，优先补充/修订 Skill。

### 1.2 环境变量

CLI 命令自动继承以下环境变量（由 AI Hub 进程注入）：

| 变量 | 说明 |
|------|------|
| `AI_HUB_SESSION_ID` | 当前会话 ID |
| `AI_HUB_GROUP_NAME` | 当前团队名 |
| `AI_HUB_PORT` | 服务端口（默认 8080） |

也可通过全局 flag 覆盖：`--session <id>` / `--group <name>` / `--port <port>`

### 1.3 三层架构

所有数据遵循三层隔离：

| 层级 | CLI --level | 作用域 | 说明 |
|------|-------------|--------|------|
| 会话级 | session | `<group>/sessions/<id>/memory` | 当前会话私有 |
| 团队级 | team | `<group>/memory` | 同团队共享 |
| 全局级 | global | `memory` | 所有会话可见 |

搜索时自动合并三层结果，优先级：会话 > 团队 > 全局。

### 1.4 记录治理

- 先搜索后写入，命中则更新，避免重复。
- 每个主题一个主文件，禁止按日期命名。
- 正文写当前状态，变更追加到「变更记录」章节。
- 禁止在正文写过程叙述。

### 1.5 诊断优先

- 遇到问题先诊断再修复，禁止盲目操作。
- 优先用 API/CLI 查询，减少直接文件操作。
- 安全重启：kill → wait → verify。

### 1.6 调度安全

- 执行类调度必须带上下文头：`[group_name|scope|target|task_id]`
- 子会话回调必须带同一 `task_id`
- 未锁定 scope/target 的执行任务不得下发

---

## §2 记忆库

### CLI 命令

```bash
# 搜索（语义匹配，默认 top_k=10）
ai-hub search "关键词" --level team
ai-hub search "关键词" --level session --top 5 --tags "标签1,标签2"

# 列出文件（含预览、创建/更新时间）
ai-hub list --level team
ai-hub list --level global

# 读取
ai-hub read "文件名.md" --level team

# 写入（--content 或 stdin）
ai-hub write "文件名.md" --level team --content "内容"
echo "内容" | ai-hub write "文件名.md" --level team

# 编辑（查找替换 + diff 输出）
ai-hub edit "文件名.md" --level team --old "旧文本" --new "新文本"

# 删除
ai-hub delete "文件名.md" --level team
ai-hub delete "文件名.md" --level team --force
```

### 搜索规则

- 搜索自动合并三层结果（session + team + global）
- 结果按相似度排序，包含文件名、预览、创建/更新时间
- 向量引擎未就绪时，回退到 list + read

### 向量引擎健康

```bash
ai-hub status    # 查看向量引擎状态
```

如果向量引擎异常，可通过 API 重启：
```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/restart
```

### 结构化记忆（mem 子命令）

```bash
# 写入结构化记忆
echo '{"type":"procedure","title":"Deploy SOP",...}' | ai-hub mem add

# 语义搜索 + 统计重排
ai-hub mem retrieve --query "deploy" --types procedure

# 反馈成功/失败
ai-hub mem feedback --id mem_20260305_0001 --result success

# 修订记忆
ai-hub mem revise --id mem_20260305_0001

# 废弃记忆
ai-hub mem deprecate --id mem_20260305_0001

# 查看 JSON Schema
ai-hub mem spec add
```

---

## §3 会话管理

### CLI 命令

```bash
# 列出所有会话（含状态：idle/streaming/alive）
ai-hub sessions

# 查看会话详情
ai-hub sessions 25

# 查看会话最近消息（默认 20 条）
ai-hub sessions 25 messages
ai-hub sessions 25 messages --limit 50

# 发消息（session_id=0 创建新会话）
ai-hub send 25 "你好"
ai-hub send 0 "初始化" --group "团队A" --work-dir "/path/to/project"
```

### 调度模式

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| 串行 | 逐个发送，等回调再发下一个 | 有依赖的任务链 |
| 并行 | 同时发送多个，各自回调 | 独立子任务 |
| 主从 | 主会话分发，从会话回报 | 团队协作 |

### 回调协议

- 子会话完成后必须主动回报，包含：执行结果 + 关键变更 + 是否需后续操作
- 异步派发：发完消息即继续，禁止轮询等待

---

## §4 规则管理

### CLI 命令

```bash
# 读取当前会话规则（自动使用 AI_HUB_SESSION_ID）
ai-hub rules get

# 读取指定会话规则
ai-hub rules get 25

# 写入会话规则
ai-hub rules set 25 --content "你是技术维护工程师"

# 删除会话规则
ai-hub rules delete 25
```

### 三层规则体系

| 层级 | 路径 | 说明 |
|------|------|------|
| 全局 | `~/.ai-hub/rules/CLAUDE.md` | 模板文件，支持 `{{VAR}}` 占位符 |
| 团队 | `~/.ai-hub/teams/<group>/rules/*.md` | 团队私有，通过向量 API 读取 |
| 会话 | `~/.ai-hub/session-rules/{id}.md` | 每会话角色定义，优先级最高 |

规则在进程启动时注入，修改后需重启进程生效。

---

## §5 笔记管理

### CLI 命令

```bash
# 列出所有笔记
ai-hub notes list

# 读取笔记
ai-hub notes read todo.md

# 写入笔记
ai-hub notes write todo.md --content "# TODO\n- item 1"

# 删除笔记
ai-hub notes delete todo.md
```

笔记存储在 `~/.ai-hub/notes/`，禁止直接 Edit/Write 该目录，必须通过 CLI 或 API 操作。

---

## §6 定时器

### CLI 命令

```bash
# 列出所有定时器
ai-hub triggers list

# 按会话筛选
ai-hub triggers list --session 25

# 创建定时器（max-fires: -1=无限, 1=一次, N=N次）
ai-hub triggers create --session 25 --time "09:00:00" --content "早报" --max-fires -1

# 更新定时器
ai-hub triggers update 1 --content "新指令"
ai-hub triggers update 1 --time "10:00:00" --enabled false

# 删除定时器
ai-hub triggers delete 1
```

### 时间格式

| 格式 | 示例 | 说明 |
|------|------|------|
| 精确时间 | `2026-03-06 09:00:00` | 一次性触发 |
| 每日时间 | `09:00:00` | 每天触发 |
| 间隔 | `1h30m` | 周期触发 |

### 指令编写原则

- 指令必须自包含，不依赖上下文
- 包含完整路径和错误处理
- 所有时间使用 UTC+8

---

## §7 系统诊断

### CLI 命令

```bash
# 版本信息
ai-hub version

# 系统状态（服务 + 向量引擎 + 进程池）
ai-hub status
```

### 常见故障排查

| 症状 | 检查命令 | 处理 |
|------|----------|------|
| 向量搜索无结果 | `ai-hub status` | 检查向量引擎状态，必要时重启 |
| 会话无响应 | `ai-hub sessions <id>` | 检查进程状态 |
| 规则未生效 | `ai-hub rules get <id>` | 确认规则内容，重启进程 |

### 日志分析

```bash
# 查看最近日志
tail -50 ~/.ai-hub/logs/ai-hub.log

# 搜索错误
grep -i "error\|forbidden\|timeout" ~/.ai-hub/logs/ai-hub.log | tail -20
```

---

## §8 脚本引擎

### 规范

多步重复操作（≥3 步）必须脚本化，禁止逐步单条交互。

### 脚本仓库

```
~/.ai-hub/scripts/
├── INDEX.md          # 脚本索引（必须维护）
├── shell/            # 系统运维脚本
├── browser/          # Chrome MCP 自动化脚本
└── api/              # HTTP 批量请求脚本
```

### 执行流程

1. 查 INDEX.md 是否有可复用脚本
2. 有 → 传参执行
3. 无 → 新建脚本 → 执行 → 更新 INDEX.md

### 脚本规范

- 命名：`<动作>-<对象>.<扩展名>`（如 `upgrade-production.sh`）
- Shell：`set -euo pipefail`
- JS：try/catch
- Python：sys.exit()
- 禁止硬编码 URL/端口/ID，全部参数化
- 失败时修复脚本重跑，禁止回退到手动操作
