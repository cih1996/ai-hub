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

## 服务管理（HTTP API，无 CLI 子命令）

服务通过 HTTP API 管理，日志自动分配到 ~/.ai-hub/logs/service-<name>.log

```bash
# 创建服务
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/services \
  -H "Content-Type: application/json" \
  -d '{"name":"项目名","command":"npm run dev","work_dir":"/path/to/project","port":3000,"auto_start":false}'

# 列出所有服务（含实时状态）
curl http://localhost:$AI_HUB_PORT/api/v1/services

# 查看单个服务
curl http://localhost:$AI_HUB_PORT/api/v1/services/<id>

# 启动 / 停止 / 重启
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/services/<id>/start
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/services/<id>/stop
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/services/<id>/restart

# 查看日志（默认100行，可指定 ?lines=200）
curl http://localhost:$AI_HUB_PORT/api/v1/services/<id>/logs?lines=50

# 更新配置（仅传需要改的字段）
curl -X PUT http://localhost:$AI_HUB_PORT/api/v1/services/<id> \
  -H "Content-Type: application/json" \
  -d '{"port":3001,"auto_start":true}'

# 删除服务（自动停止运行中的进程）
curl -X DELETE http://localhost:$AI_HUB_PORT/api/v1/services/<id>
```

字段说明：name(必填), command(必填), work_dir, port, auto_start(启动时自动运行)
状态值：stopped / running / dead

## 系统

ai-hub version                           # 版本
ai-hub status                            # 系统状态
