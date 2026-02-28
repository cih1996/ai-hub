---
name: "一号脚本引擎"
description: "AI Hub 脚本批量化执行引擎。当需要执行多步重复操作（≥3步）、浏览器自动化、批量API调用、系统运维操作时触发。要求先查索引复用已有脚本，无则新建脚本再执行，禁止逐步单条交互。脚本支持参数化、跨会话复用、失败修复重跑。"
---

# 脚本引擎 — 批量化执行手册

## 核心理念

**多步操作必须脚本化，禁止逐步单条执行。** 脚本可复用、可传参、可修复，新会话通过索引即可继承能力。

## 脚本仓库

```
~/.ai-hub/scripts/
├── browser/     # JS 脚本（Chrome MCP evaluate_script 执行）
├── shell/       # Shell 脚本（bash/zsh）
├── api/         # HTTP 批量请求脚本（curl/python requests）
└── INDEX.md     # 全局索引：脚本名 + 用途 + 参数说明
```

## 执行流程

### 1. 先查后做（每次任务必须）

```bash
cat ~/.ai-hub/scripts/INDEX.md
```

找到匹配脚本 → 读头部了解参数 → 传参执行。

### 2. 无脚本则新建

判断条件：≥3 步的重复性操作，或预期未来会再次执行的操作。

创建流程：
1. 写脚本到对应分类目录
2. 本地测试通过
3. 更新 INDEX.md

### 3. 失败修复

脚本执行失败时：修复脚本 → 重跑验证 → 记录修改原因到脚本注释。
**禁止放弃脚本改回手动操作。**

---

## 脚本规范

### 文件头（必须）

```bash
#!/bin/bash
# 描述: 一句话说明脚本用途
# 参数: $1=xxx $2=yyy（无参数写「无」）
# 用法: bash ~/.ai-hub/scripts/shell/xxx.sh <参数>
# 示例: bash ~/.ai-hub/scripts/shell/xxx.sh v1.0.0 "修复说明"
```

JS 脚本：
```javascript
// 描述: 一句话说明
// 参数: url=目标地址, keyword=搜索词
// 用法: 通过 Chrome MCP evaluate_script 执行
// 示例: evaluate_script({ function: scriptContent, args: [...] })
```

### 命名规范

`<动作>-<对象>.<扩展名>`

| 示例 | 说明 |
|------|------|
| upgrade-production.sh | 升级生产实例 |
| scan-issues.sh | 扫描GitHub Issues |
| batch-send-messages.py | 批量发送消息 |
| fill-form.js | 浏览器批量填表 |
| deploy-feishu-app.js | 飞书应用部署自动化 |

### 编码要求

- Shell: `set -euo pipefail`，退出码 0=成功 非0=失败
- JS: `try/catch` 包裹，返回 `{success: bool, error: string}`
- Python: `if __name__ == "__main__"` + `sys.exit()`
- 禁止硬编码 URL、端口、ID，全部参数化
- 涉及中文的 HTTP 请求（Windows）：写临时文件用 `curl -d @file`

### 跨平台

涉及 Windows 的脚本优先用 Python 统一，或同时提供 `.sh` + `.bat` 版本。

---

## 分类指南

### shell/ — 系统运维

适用场景：编译部署、进程管理、Git操作、文件批处理

```bash
# 执行示例
bash ~/.ai-hub/scripts/shell/upgrade-production.sh v1.78.0 "修复说明"
bash ~/.ai-hub/scripts/shell/scan-issues.sh
```

### browser/ — 浏览器自动化

适用场景：Chrome MCP 操作网页、表单填写、截图验证、第三方平台操作

```javascript
// 写法：导出为可传参的函数字符串
// 执行：通过 evaluate_script 注入页面执行
// 复杂流程：拆分为多个步骤脚本，按顺序调用

// 示例：批量操作页面元素
(selector, action) => {
  const elements = document.querySelectorAll(selector);
  const results = [];
  elements.forEach(el => {
    try { el[action](); results.push({ok: true}); }
    catch(e) { results.push({ok: false, error: e.message}); }
  });
  return results;
}
```

### api/ — HTTP 批量请求

适用场景：批量API调用、数据迁移、多会话消息分发

```python
#!/usr/bin/env python3
# 描述: 批量向多个会话发送消息
# 参数: --port=服务端口 --sessions=会话ID列表 --message=消息内容
# 用法: python3 ~/.ai-hub/scripts/api/batch-send-messages.py --port 8080 --sessions 21,23,25 --message "通知内容"

import argparse, requests, json, sys

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', default='8080')
    parser.add_argument('--sessions', required=True)
    parser.add_argument('--message', required=True)
    args = parser.parse_args()

    for sid in args.sessions.split(','):
        resp = requests.post(
            f'http://localhost:{args.port}/api/v1/chat/send',
            json={'session_id': int(sid), 'content': args.message}
        )
        print(f'session {sid}: {resp.status_code}')

if __name__ == '__main__':
    main()
```

---

## INDEX.md 维护

每次新建/删除/重命名脚本后，必须同步更新 `~/.ai-hub/scripts/INDEX.md`。

格式：

```markdown
## shell/
| 脚本 | 用途 | 参数 |
|------|------|------|
| upgrade-production.sh | 升级本地生产实例 | $1=版本号 $2=Release说明 |

## browser/
| 脚本 | 用途 | 参数 |
|------|------|------|

## api/
| 脚本 | 用途 | 参数 |
|------|------|------|
```
