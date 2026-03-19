# AI Hub CLI 速查表

环境变量自动继承：AI_HUB_SESSION_ID / AI_HUB_GROUP_NAME / AI_HUB_PORT

## 记忆库（--level 可选：session / team / global）

ai-hub list --level <level>                          # 列出记忆文件
ai-hub search "关键词" [--level <level>] [--top 10]  # 语义搜索（不带 --level 时全量搜索三层）
ai-hub read "文件名.md" --level <level>              # 读取全文
ai-hub write "文件名.md" --level <level> --content "内容"  # 写入
ai-hub edit "文件名.md" --level <level> --old "旧" --new "新"  # diff修改
ai-hub delete "文件名.md" --level <level> --force    # 删除

level 解析：session 需要 GROUP_NAME+SESSION_ID，team 需要 GROUP_NAME，global 无需。
search 返回字段：level（来源层级）、snippet（匹配片段）、hit_count（命中次数）、read_count（阅读次数）。
写入规范：先搜后写，一主题一文件，正文写当前有效状态，历史追加到变更记录区。

## 会话管理

ai-hub sessions                          # 列出所有会话（含错误统计 E:n W:n）
ai-hub sessions --with-errors            # 只显示有错误/警告的会话
ai-hub sessions <id>                     # 会话详情+状态
ai-hub sessions <id> messages            # 最近消息
ai-hub sessions <id> move --group <name> # 将会话移动到指定团队
ai-hub send <session_id> "消息内容"       # 发消息（0=新建会话）
ai-hub send <session_id> "消息" --remote <url>  # 发送到远程实例（跨系统协作）

## 错误统计

ai-hub errors                            # 所有会话错误统计概览
ai-hub errors <session_id>               # 查看会话的错误列表
ai-hub errors <session_id> --level error # 只看错误（不含警告）
ai-hub errors <session_id> --context <message_id>  # 查看出错消息的上下文
ai-hub errors <session_id> --context <message_id> --lines 5  # 指定上下文行数（默认2）

用途：AI 可通过 --context 拉取犯错时的对话上下文，分析错误原因。

## 规则管理

ai-hub rules get [session_id]            # 读取会话规则（默认当前）
ai-hub rules set <session_id> --content "内容"  # 写入会话规则
ai-hub rules delete <session_id>         # 删除会话规则

# 团队/全局规则（基于文件）
ai-hub rules list --level <global|team>  # 列出规则文件
ai-hub rules get <filename.md> --level <global|team>  # 读取规则文件
ai-hub rules set <filename.md> --level <global|team> --content "内容"  # 写入规则文件
ai-hub rules delete <filename.md> --level <global|team>  # 删除规则文件

level 解析：team 需要 GROUP_NAME，global 无需。

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

## 文件传输

ai-hub transfer send --file <路径> --remote <地址> [--save <远程保存路径>]  # 发送文件到远程实例
ai-hub transfer pull --remote <地址> --id <ID> --save <本地保存路径>        # 从远程拉取文件
ai-hub transfer list [--remote <地址>]                                      # 列出传输记录
ai-hub transfer status <ID> [--remote <地址>]                               # 查看传输进度
ai-hub transfer delete <ID> [--remote <地址>]                               # 删除传输记录

## 系统

ai-hub version                           # 版本
ai-hub status                            # 系统状态

## 守护进程管理

ai-hub daemon start      # 启动服务
ai-hub daemon stop       # 优雅停止（通过 API，非 kill）
ai-hub daemon restart    # 重启
ai-hub daemon install    # 手动安装为系统服务
ai-hub daemon uninstall  # 卸载系统服务
ai-hub daemon status     # 状态

安装位置：
- macOS: /usr/local/bin/ai-hub + launchd
- Linux: ~/.local/bin/ai-hub + systemd --user
- Windows: %LOCALAPPDATA%\ai-hub\ai-hub.exe + 启动文件夹快捷方式

## 热重载

ai-hub reload vector                  # 重载向量模型（使用本地缓存）
ai-hub reload vector --force-download # 强制重新下载模型
ai-hub reload config                  # 重载配置（预留）
ai-hub reload skills                  # 重载 Skill（预留）

向量引擎故障排查：
1. 查看状态：ai-hub status（检查 vector 部分）
2. 查看日志：tail -100 ~/.ai-hub/logs/ai-hub.log | grep vector
3. 强制重下载：ai-hub reload vector --force-download
