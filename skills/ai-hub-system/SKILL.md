---
name: "一号系统感知"
description: "AI Hub 系统自感知 Skill。当用户提及查看日志、系统出错、调用系统能力、监控会话、进程管理、端口占用、项目仓库等系统级话题时触发。提供日志位置、API 清单、项目仓库、进程管理等关键信息。"
---

# 系统自感知 — AI Hub 运维手册

## 一、日志

路径：`~/.ai-hub/logs/ai-hub.log`
特性：每次服务启动时自动清空旧日志，目录不存在时自动创建。

常用操作：
```bash
# 实时查看日志
tail -f ~/.ai-hub/logs/ai-hub.log

# 搜索错误
grep -i error ~/.ai-hub/logs/ai-hub.log

# 查看最近 50 行
tail -50 ~/.ai-hub/logs/ai-hub.log
```

## 二、系统 API 接口清单

基础地址：`http://localhost:$AI_HUB_PORT`
请求头：`Content-Type: application/json`

### Providers（模型供应商）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/providers | 列出所有供应商 |
| POST | /api/v1/providers | 创建供应商 |
| PUT | /api/v1/providers/:id | 更新供应商 |
| DELETE | /api/v1/providers/:id | 删除供应商 |

### Sessions（会话）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/sessions | 列出所有会话 |
| POST | /api/v1/sessions | 创建会话 |
| GET | /api/v1/sessions/:id | 获取会话详情 |
| PUT | /api/v1/sessions/:id | 更新会话 |
| DELETE | /api/v1/sessions/:id | 删除会话 |
| GET | /api/v1/sessions/:id/messages | 获取会话消息 |
| POST | /api/v1/sessions/:id/compress | 压缩会话上下文 |
| POST | /api/v1/sessions/:id/restart | 重启会话 |

### Session Rules（会话规则）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/session-rules/:id | 获取会话规则 |
| PUT | /api/v1/session-rules/:id | 更新会话规则 |
| DELETE | /api/v1/session-rules/:id | 删除会话规则 |

### Chat（对话）
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/chat/send | 发送消息（session_id=0 创建新会话） |
| WS | /ws/chat | WebSocket 实时通信 |

### Files（文件管理）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/files | 列出文件 |
| GET | /api/v1/files/content | 读取文件内容 |
| PUT | /api/v1/files/content | 写入文件内容 |
| POST | /api/v1/files | 创建文件 |
| DELETE | /api/v1/files | 删除文件 |
| GET | /api/v1/files/variables | 获取模板变量 |
| GET | /api/v1/files/default | 获取默认文件内容 |

### Project Rules（项目规则）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/project-rules | 列出项目规则 |
| GET | /api/v1/project-rules/content | 读取项目规则 |
| PUT | /api/v1/project-rules/content | 写入项目规则 |

### Status（系统状态）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/status | 获取系统状态和依赖检查 |
| POST | /api/v1/status/retry-install | 重试安装依赖 |
| GET | /api/v1/version | 获取当前版本号 |

### Skills（技能）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/skills | 列出所有 Skill |
| POST | /api/v1/skills/toggle | 启用/禁用 Skill |

### MCP（模型上下文协议）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/mcp | 列出 MCP 配置 |
| POST | /api/v1/mcp/toggle | 启用/禁用 MCP |

### Triggers（定时触发器）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/triggers | 列出触发器 |
| POST | /api/v1/triggers | 创建触发器 |
| PUT | /api/v1/triggers/:id | 更新触发器 |
| DELETE | /api/v1/triggers/:id | 删除触发器 |

### Vector（向量知识库）
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/vector/search | 统一语义搜索（body: {query, scope, top_k}） |
| POST | /api/v1/vector/search_knowledge | 语义搜索知识库 |
| POST | /api/v1/vector/search_memory | 语义搜索记忆库 |
| POST | /api/v1/vector/read_knowledge | 读取知识文件 |
| POST | /api/v1/vector/read_memory | 读取记忆文件 |
| POST | /api/v1/vector/write_knowledge | 写入知识文件 |
| POST | /api/v1/vector/write_memory | 写入记忆文件 |
| POST | /api/v1/vector/delete_knowledge | 删除知识文件 |
| POST | /api/v1/vector/delete_memory | 删除记忆文件 |
| GET | /api/v1/vector/stats | 向量命中统计 |
| GET | /api/v1/vector/status | 向量引擎状态 |
| POST | /api/v1/vector/restart | 重启向量引擎 |

### Channels（通讯频道）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/channels | 列出所有频道 |
| POST | /api/v1/channels | 创建频道 |
| PUT | /api/v1/channels/:id | 更新频道 |
| DELETE | /api/v1/channels/:id | 删除频道 |
| POST | /api/v1/webhook/feishu | 飞书 Webhook 回调 |
| POST | /api/v1/webhook/qq | QQ Webhook 回调（OneBot 11），支持 ?channel_id= 指定频道 |

## 三、项目仓库

地址：https://github.com/cih1996/ai-hub

遇到无法本地修复的问题时，引导用户提 Issue：
```bash
gh issue create --repo cih1996/ai-hub --title "问题标题" --body "问题描述"
```

## 四、进程管理

```bash
# 查看 AI Hub 进程
ps aux | grep ai-hub

# 查看端口占用
lsof -i:$AI_HUB_PORT

# 查看当前版本
curl -s http://localhost:$AI_HUB_PORT/api/v1/version
```
