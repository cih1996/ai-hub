# AI Hub CLI 速查表

环境变量自动继承：AI_HUB_SESSION_ID / AI_HUB_GROUP_NAME / AI_HUB_PORT

## 记忆库（--level 必填：session / team / global）

ai-hub list --level <level>                          # 列出记忆文件
ai-hub search "关键词" --level <level> [--top 10]    # 语义搜索
ai-hub read "文件名.md" --level <level>              # 读取全文
ai-hub write "文件名.md" --level <level> --content "内容"  # 写入
ai-hub edit "文件名.md" --level <level> --old "旧" --new "新"  # diff修改
ai-hub delete "文件名.md" --level <level> --force    # 删除

level 解析：session 需要 GROUP_NAME+SESSION_ID，team 需要 GROUP_NAME，global 无需。
写入规范：先搜后写，一主题一文件，正文写当前有效状态，历史追加到变更记录区。

## 会话管理

ai-hub sessions                          # 列出所有会话
ai-hub sessions <id>                     # 会话详情+状态
ai-hub sessions <id> messages            # 最近消息
ai-hub send <session_id> "消息内容"       # 发消息（0=新建会话）

## 规则管理

ai-hub rules get [session_id]            # 读取会话规则（默认当前）
ai-hub rules set <session_id> --content "内容"  # 写入会话规则
ai-hub rules delete <session_id>         # 删除会话规则

## 笔记管理

ai-hub notes list                        # 列出笔记
ai-hub notes read <filename>             # 读取笔记
ai-hub notes write <filename> --content "内容"  # 写入笔记
ai-hub notes delete <filename>           # 删除笔记

## 定时器

ai-hub triggers list                     # 列出定时器
ai-hub triggers create --session <id> --time "09:00:00" --content "指令" [--max-fires -1]
ai-hub triggers update <id> --content "新指令"
ai-hub triggers delete <id>              # 删除

## 服务管理

ai-hub services                          # 列出所有服务（含实时状态）
ai-hub services <id|name>                # 查看服务详情
ai-hub services create --name "名称" --cmd "命令" [--dir /path] [--svc-port 3000] [--auto-start]
ai-hub services start <id|name>          # 启动服务
ai-hub services stop <id|name>           # 停止服务
ai-hub services restart <id|name>        # 重启服务
ai-hub services logs <id|name> [--lines 50]  # 查看日志（默认100行）
ai-hub services delete <id|name>         # 删除服务（自动停止运行中的进程）

字段说明：name(必填), cmd(必填), dir(工作目录), svc-port(服务端口), auto-start(启动时自动运行)
状态值：stopped / running / dead

## 系统

ai-hub version                           # 版本
ai-hub status                            # 系统状态
