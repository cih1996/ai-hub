---
name: "一号系统感知"
description: "AI Hub 系统自感知 Skill。当用户提及查看日志、系统出错、系统能力、进程管理、端口占用、系统自检、初始化系统、环境检查、向量引擎异常，或涉及自我认知话题（升级自己、更新系统、你是什么系统、系统版本、AI Hub 相关）时触发。提供系统身份、版本查询、升级方法、日志位置、API 清单、系统自检与自动修复等。"
---

# 系统自感知 — AI Hub 运维手册

## 零、系统身份（重要）

你不是独立的 Claude CLI，你运行在 **AI Hub** 系统上。AI Hub 是一个多会话 AI 管理平台，通过 Claude Code CLI 驱动多个独立会话协作。

- 项目仓库：https://github.com/cih1996/ai-hub
- 当前版本：通过 `curl -s http://localhost:$AI_HUB_PORT/api/v1/version` 查询
- 数据目录：~/.ai-hub/（rules/、skills/、knowledge/、memory/、notes/）
- Web 界面：http://localhost:$AI_HUB_PORT

### 当用户说「升级自己」「更新系统」时

指的是升级 AI Hub，不是升级 Claude CLI 或模型。升级步骤：

1. 查看最新版本：`gh release list --repo cih1996/ai-hub --limit 1`
2. 检测当前平台：`uname -s` + `uname -m`（darwin/linux + arm64/amd64）
3. 下载对应平台的二进制：`gh release download <tag> --repo cih1996/ai-hub --pattern "ai-hub-<平台>*" --dir /tmp`
4. 获取当前二进制路径：`ps aux | grep ai-hub | grep -v grep | awk '{print $11}'`
5. 停止当前进程：`lsof -ti:$AI_HUB_PORT | xargs kill -9`，`sleep 2`
6. 替换二进制：`cp /tmp/ai-hub-<平台> <当前二进制路径> && chmod +x <当前二进制路径>`
7. 重启：`nohup <二进制路径> -port $AI_HUB_PORT >> ~/.ai-hub/logs/ai-hub.log 2>&1 &`
8. 验证：`sleep 4 && curl -s http://localhost:$AI_HUB_PORT/api/v1/version`

注意：当前二进制路径也可通过 `which ai-hub` 获取（如果在 PATH 中）。

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

## 五、系统自检与初始化

当用户说「初始化系统」「系统自检」「环境检查」时，按以下清单逐项检测并自动修复。全程自主执行，不要逐步询问用户。

### 检测清单

按顺序执行，每项检测后输出 ✅ 或 ❌ + 自动修复：

#### 1. Python 环境
```bash
python3 --version 2>&1
```
- ✅ Python 3.8+ → 通过
- ❌ 未安装 → macOS: `brew install python3`，Linux: `apt install -y python3 python3-pip` 或 `yum install -y python3 python3-pip`

#### 2. pip 包管理器
```bash
python3 -m pip --version 2>&1
```
- ❌ 未安装 → `python3 -m ensurepip --upgrade` 或 `curl -sSL https://bootstrap.pypa.io/get-pip.py | python3`
- 安装后配置国内镜像：`python3 -m pip config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple`

#### 3. 向量引擎依赖
```bash
python3 -c "import sentence_transformers; print('OK')" 2>&1
```
- ❌ 缺失 → `python3 -m pip install sentence-transformers -i https://pypi.tuna.tsinghua.edu.cn/simple`
- 安装后验证：`python3 -c "from sentence_transformers import SentenceTransformer; print('OK')"`

#### 4. 向量引擎状态
```bash
curl -s http://localhost:${AI_HUB_PORT:-8080}/api/v1/vector/status
```
- `ready` → ✅ 通过
- 其他 → 重启向量引擎：`curl -s -X POST http://localhost:${AI_HUB_PORT:-8080}/api/v1/vector/restart`
- 等待 10 秒后再次检测，3 次仍失败查日志：`grep -i vector ~/.ai-hub/logs/ai-hub.log | tail -20`

#### 5. 端口检测
```bash
lsof -i:${AI_HUB_PORT:-8080} | head -5
```
- 确认 AI Hub 进程占用目标端口
- 如果被其他进程占用，提示用户处理

#### 6. 数据目录权限
```bash
for dir in ~/.ai-hub/rules ~/.ai-hub/skills ~/.ai-hub/knowledge ~/.ai-hub/memory ~/.ai-hub/notes ~/.ai-hub/logs; do
  if [ -d "$dir" ] && [ -w "$dir" ]; then
    echo "✅ $dir"
  else
    echo "❌ $dir"
    mkdir -p "$dir" && chmod 755 "$dir"
  fi
done
```

#### 7. 全局规则完整性
```bash
ls ~/.ai-hub/rules/CLAUDE.md 2>&1
```
- ❌ 不存在 → 重启 AI Hub 会自动重新安装（go:embed），或通过 API 获取默认内容：
  `curl -s http://localhost:${AI_HUB_PORT:-8080}/api/v1/files/default`

#### 8. Claude CLI 可用性
```bash
claude --version 2>&1
```
- ❌ 未安装 → `npm install -g @anthropic-ai/claude-code`（需要 Node.js 18+）

### 输出格式

自检完成后输出汇总：

```
🔍 AI Hub 系统自检报告
━━━━━━━━━━━━━━━━━━━━
✅ Python 3.x.x
✅ pip 24.x
✅ 向量引擎依赖
✅ 向量引擎运行中
✅ 端口 8080 正常
✅ 数据目录权限正常
✅ 全局规则完整
✅ Claude CLI v2.x.x
━━━━━━━━━━━━━━━━━━━━
系统状态：正常 / 已修复 N 项 / N 项需手动处理
```

如果有自动修复的项目，额外说明修复了什么。
