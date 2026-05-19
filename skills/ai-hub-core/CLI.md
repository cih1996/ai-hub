# 一号核心手册 · CLI

> 本文档只记录 CLI 用法。
> 真实实现以 `cli/main.go` 与 `cli/commands/*.go` 为准。
> 大多数 CLI 都是 `/api/v1` 的薄封装；对应关系见同目录 `HTTP.md`。

---

## 1. 全局参数

所有命令都支持以下全局参数，可写在命令前：

```bash
ai-hub --session 25 --group "团队A" --port 9527 <command>
```

| 参数 | 说明 |
|------|------|
| `--session <id>` | 会话 ID，默认读取 `AI_HUB_SESSION_ID` |
| `--group <name>` | 团队名，默认读取 `AI_HUB_GROUP_NAME` |
| `--port <port>` | 服务端口，默认读取 `AI_HUB_PORT`，缺省为 `9527` |
| `--help` | 显示帮助 |
| `--version` | 显示版本 |

---

## 2. 使用原则

- 记忆库、知识库、团队/全局规则优先用文件系统工具直接读写。
- 需要复用现成 CLI 行为、跨机调用或确认真实服务状态时，再用 CLI。
- 大多数命令会命中本地 `/api/v1`；少数例外：
  - `rules --level global|team` 直接读写 `~/.ai-hub` 下的规则文件
  - `daemon install/uninstall/start/stop` 主要通过系统服务管理器操作
  - `transfer` 直接请求目标 AI Hub 的 HTTP 接口

---

## 3. 已补齐的旧手册遗漏项

以下 CLI 在旧版 `SKILL.md` 中缺失或记录不完整，本文件已补齐：

- `services`
- `errors`
- `daemon`
- `reload`
- `skills`
- `schemas`
- `transfer`
- `changelog`
- `sessions health`
- `sessions reset`
- `sessions messages` 的分页、搜索、上下文查看
- `rules list/get/set/delete` 的团队级与全局级文件模式

---

## 4. 记忆库命令

这些命令面向 `memory` 三层数据。

### 4.1 搜索与读取

```bash
# 搜索，默认合并 session + team + global
ai-hub search "部署流程"

# 限定层级
ai-hub search "BUG修复" --level session --top 5

# 列表
ai-hub list --level session
ai-hub list --level team
ai-hub list --level global

# 读取
ai-hub read "team-sop.md" --level team
```

### 4.2 写入与编辑

```bash
# 覆盖写入
ai-hub write "note.md" --level session --content "# My Note"

# 从 stdin 写入
echo "hello" | ai-hub write "note.md" --level team

# 文本替换
ai-hub edit "note.md" --level session --old "旧内容" --new "新内容"

# 删除
ai-hub delete "old-note.md" --level session --force
```

说明：

- `--level` 必填：`session` / `team` / `global`
- `write --schema` 仍被解析，但当前文件式记忆写入已忽略该参数
- `search` 省略 `--level` 时会自动合并三层结果

---

## 5. 会话与消息

### 5.1 基础会话管理

```bash
# 会话列表
ai-hub sessions
ai-hub sessions --group "团队A"
ai-hub sessions --with-errors

# 会话详情
ai-hub sessions 25

# 迁移团队
ai-hub sessions 25 move --group "新团队"
ai-hub sessions 25 move --group ""
```

### 5.2 消息查看

```bash
# 最近消息
ai-hub sessions 25 messages
ai-hub sessions 25 messages --limit 50

# 分页
ai-hub sessions 25 messages --page 2 --size 20

# 搜索
ai-hub sessions 25 messages --search "timeout"

# 统计总条数
ai-hub sessions 25 messages --count

# 读取第 N 条消息的上下文
ai-hub sessions 25 messages --nth 30

# 从某个 message_id 查看上下文
ai-hub sessions 25 messages --from 1024
```

### 5.3 会话健康度与重置

```bash
# 查看健康度
ai-hub sessions 25 health

# 设置健康度
ai-hub sessions 25 health --set green

# 递增计数器
ai-hub sessions 25 health --incr correction_count
ai-hub sessions 25 health --incr drift_count

# 重置上下文
ai-hub sessions 25 reset --yes
ai-hub sessions 25 reset --keep-last 20 --yes
```

### 5.4 发消息

```bash
# 发给已有会话
ai-hub send 25 "你好"

# session_id=0 表示新建会话
ai-hub send 0 "初始化" --group "团队A" --work-dir "/path/to/project"

# 发往远端 AI Hub
ai-hub send 23 "跨系统协作" --remote http://192.168.1.100
```

---

## 6. 错误诊断

```bash
# 全局错误统计
ai-hub errors --all

# 某会话的错误/警告
ai-hub errors 25
ai-hub errors 25 --level error
ai-hub errors 25 --level warning

# 查看某条报错消息的上下文
ai-hub errors 25 --context 1024 --lines 3
```

说明：

- `errors` 默认可用于看统计概览
- `--context` 会进一步读取该消息前后上下文

---

## 7. 团队与规则

### 7.1 团队

```bash
ai-hub groups
ai-hub groups "团队名"
ai-hub groups create "团队名" --desc "团队描述"
ai-hub groups delete "团队名"
```

### 7.2 规则

会话级规则走 HTTP；团队级/全局级规则走本地文件系统。

```bash
# 会话级
ai-hub rules get
ai-hub rules get 25
ai-hub rules set 25 --content "你是技术维护工程师"
ai-hub rules delete 25

# 团队级
ai-hub rules list --level team
ai-hub rules get team-rules.md --level team
ai-hub rules set team-rules.md --level team --content "团队规则内容"
ai-hub rules delete team-rules.md --level team

# 全局级
ai-hub rules list --level global
ai-hub rules get CLAUDE.md --level global
ai-hub rules set custom.md --level global --content "全局规则内容"
ai-hub rules delete custom.md --level global
```

---

## 8. 笔记文件、定时器、钩子

### 8.1 笔记文件

- `notes` 专用 CLI 已移除，不再提供 `ai-hub notes *`
- AI 处理笔记时直接读写 `~/.ai-hub/notes/`
- 需要通过服务接口访问时，直接走通用文件 API：`scope=notes`

### 8.2 定时器

```bash
ai-hub triggers list
ai-hub triggers list --session 25
ai-hub triggers create --session 25 --time "09:00:00" --content "早报" --max-fires -1
ai-hub triggers update 1 --content "新指令"
ai-hub triggers update 1 --time "10:00:00" --enabled false
ai-hub triggers delete 1
```

时间格式：

- `2026-03-06 09:00:00`：一次性触发
- `09:00:00`：每日触发
- `1h30m`：间隔触发

### 8.3 事件钩子

```bash
ai-hub hooks list
ai-hub hooks list --event message.received

ai-hub hooks create \
  --event message.received \
  --condition "content_match:紧急|urgent" \
  --target-session 5 \
  --payload "[紧急消息] 来自会话 {source_session_id}:\n{content}"

ai-hub hooks enable 1
ai-hub hooks disable 1
ai-hub hooks delete 1
```

常用模板变量：

- `{source_session_id}`
- `{event_type}`
- `{content}`
- `{message_count}`

---

## 9. Services

这是旧手册缺失最明显的一组能力。

```bash
# 列出服务
ai-hub services
ai-hub services list

# 查看详情（支持 name 或 id）
ai-hub services 1
ai-hub services my-service
ai-hub services info 1

# 创建
ai-hub services create \
  --name my-service \
  --cmd "npm run dev" \
  --dir "/path/to/project" \
  --svc-port 3000 \
  --auto-start

# 启停重启
ai-hub services start 1
ai-hub services stop my-service
ai-hub services restart 1

# 日志
ai-hub services logs 1 --lines 200

# 删除
ai-hub services delete 1
```

说明：

- `services` 支持名称或数字 ID 解析
- 创建时 `--command` 与 `--cmd` 等价，`--work-dir` 与 `--dir` 等价
- 服务状态常见值：`running` / `dead` / 其他空闲状态

---

## 10. Skills 与 Schemas

### 10.1 Skills

```bash
ai-hub skills
ai-hub skills list
ai-hub skills read ai-hub-core
ai-hub skills create my-skill --content "---\nname: my-skill\n---\n# My Skill"
ai-hub skills update my-skill --content "updated content"
ai-hub skills delete my-skill
```

### 10.2 Schemas

```bash
ai-hub schemas list
ai-hub schemas get my-schema
ai-hub schemas create my-schema --definition '{"type":"object"}'
ai-hub schemas create my-schema --definition '{"type":"object"}' --writers '[21,23]'
cat schema.json | ai-hub schemas create my-schema
ai-hub schemas delete my-schema
```

说明：

- `schemas create` 支持 `--definition` 或 stdin
- `--writers` 需要传 JSON 数组

---

## 11. 系统运维

### 11.1 状态、版本、热重载

```bash
ai-hub status
ai-hub version

ai-hub reload config
ai-hub reload skills
```

### 11.2 daemon

```bash
ai-hub daemon start
ai-hub daemon stop
ai-hub daemon restart
ai-hub daemon install
ai-hub daemon uninstall
ai-hub daemon status
```

说明：

- `daemon` 是平台相关能力，按 macOS / Linux / Windows 走不同实现
- `daemon status` 会探测服务的 `/api/v1/version`
- `daemon stop` 在部分平台优先通过系统服务管理器停机，而不是只发 shutdown API

### 11.3 变更历史

```bash
ai-hub changelog "项目.md" --scope memory
ai-hub changelog "项目.md" --scope memory --limit 5
ai-hub changelog "项目.md" --scope memory --rollback 3
```

---

## 12. 挂载与传输

### 12.1 静态挂载

```bash
ai-hub mount ~/Pictures --alias media
ai-hub mount /tmp/screenshots --alias shots
ai-hub mount list
ai-hub mount remove media
```

访问格式：

```text
http://localhost:<port>/static/<alias>/<filename>
```

### 12.2 文件传输

```bash
# 上传到远端
ai-hub transfer send --file ./data.zip --remote http://192.168.1.100

# 从远端拉取
ai-hub transfer pull --remote http://192.168.1.100 --id <transfer_id> --save ./data.zip

# 列出远端传输记录
ai-hub transfer list --remote http://192.168.1.100

# 查看状态
ai-hub transfer status <transfer_id> --remote http://192.168.1.100

# 删除传输记录
ai-hub transfer delete <transfer_id> --remote http://192.168.1.100
```

说明：

- `transfer` 允许跨机器传文件
- `--remote` 未显式带端口时，默认补 `:9527`
- 上传使用三段式流程：初始化 -> 分块上传 -> 完成

---

## 13. 其他说明

- `mem` 结构化记忆运行时已移除，只保留历史 schema 说明。
- `knowledge` 当前没有独立 CLI，仍建议直接走文件系统工具或 HTTP 文件接口。
- 需要查每个 CLI 的真实 HTTP 映射时，继续读取 `HTTP.md`。
