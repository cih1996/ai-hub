# CLI 速查表

> 所有命令自动继承环境变量：`AI_HUB_SESSION_ID`（会话ID）、`AI_HUB_GROUP_NAME`（团队名）、`AI_HUB_PORT`（端口，默认8080）。
> 也可通过全局 flag 覆盖：`--session <id>` / `--group <name>` / `--port <port>`

## 记忆库

```bash
ai-hub search "关键词" --level <session|team|global> [--top N] [--tags "标签"]
ai-hub list --level <session|team|global>
ai-hub read "文件名.md" --level <session|team|global>
ai-hub write "文件名.md" --level <session|team|global> --content "内容"
ai-hub edit "文件名.md" --level <session|team|global> --old "旧文本" --new "新文本"
ai-hub delete "文件名.md" --level <session|team|global> [--force]
```

三层隔离：session（会话私有）> team（团队共享）> global（全局可见）。搜索自动合并三层。

使用规范：
- 写入前必须先搜索，命中则更新，避免重复
- 每个主题一个主文件，禁止按日期命名
- 正文写当前状态，变更追加到「变更记录」章节

## 会话管理

```bash
ai-hub sessions                                    # 列出所有会话
ai-hub sessions <id>                               # 会话详情
ai-hub sessions <id> messages [--limit N]          # 最近消息
ai-hub send <session_id> "消息内容"                 # 发消息（0=新建会话）
ai-hub send 0 "内容" --group "组名" --work-dir "/path"  # 新建会话
```

## 规则管理

```bash
ai-hub rules get [session_id]                      # 读取规则（默认当前会话）
ai-hub rules set <session_id> --content "内容"      # 写入规则
ai-hub rules delete <session_id>                   # 删除规则
```

## 笔记管理

```bash
ai-hub notes list                                  # 列出笔记
ai-hub notes read <filename>                       # 读取笔记
ai-hub notes write <filename> --content "内容"      # 写入笔记
ai-hub notes delete <filename>                     # 删除笔记
```

## 定时器

```bash
ai-hub triggers list [--session <id>]              # 列出定时器
ai-hub triggers create --session <id> --time "09:00:00" --content "指令" [--max-fires -1]
ai-hub triggers update <id> [--content "新指令"] [--time "10:00:00"] [--enabled true/false]
ai-hub triggers delete <id>                        # 删除定时器
```

时间格式：`2026-03-06 09:00:00`（一次性）/ `09:00:00`（每日）/ `1h30m`（间隔）

## 系统诊断

```bash
ai-hub version                                     # 版本信息
ai-hub status                                      # 系统状态
```
