---
name: "飞书消息发送"
description: "飞书消息发送接口调用指南。当 AI 需要回复飞书消息、向飞书群或用户发送消息时触发。提供获取 token 和发送消息的完整 curl 调用方式。"
---

# 飞书消息发送 — 调用手册

你收到了来自飞书的消息，需要通过飞书 API 回复。本手册提供完整的调用方式。

## 调用流程

```
1. 用 app_id + app_secret 获取 tenant_access_token
2. 用 token + chat_id 发送消息
```

## 第一步：获取 Token

```bash
curl -s -X POST 'https://open.larksuite.com/open-apis/auth/v3/tenant_access_token/internal' \
  -H 'Content-Type: application/json' \
  -d '{"app_id":"<APP_ID>","app_secret":"<APP_SECRET>"}'
```

响应：
```json
{"code":0,"msg":"ok","tenant_access_token":"t-xxx","expire":7200}
```

Token 有效期 2 小时，可缓存复用，过期后重新获取。

## 第二步：发送文本消息

```bash
curl -s -X POST 'https://open.larksuite.com/open-apis/im/v1/messages?receive_id_type=chat_id' \
  -H 'Authorization: Bearer <TENANT_ACCESS_TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{
    "receive_id": "<CHAT_ID>",
    "msg_type": "text",
    "content": "{\"text\":\"你的回复内容\"}"
  }'
```

参数说明：
- `receive_id_type`：`chat_id`（群聊）或 `open_id`（私聊指定用户）
- `receive_id`：从【飞书消息】中的「会话」字段获取（即 chat_id）
- `content`：JSON 字符串，注意内层引号需要转义

## 发送富文本消息

```bash
curl -s -X POST 'https://open.larksuite.com/open-apis/im/v1/messages?receive_id_type=chat_id' \
  -H 'Authorization: Bearer <TENANT_ACCESS_TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{
    "receive_id": "<CHAT_ID>",
    "msg_type": "post",
    "content": "{\"zh_cn\":{\"title\":\"标题\",\"content\":[[{\"tag\":\"text\",\"text\":\"正文内容\"}]]}}"
  }'
```

## 回复指定消息

如果要回复某条具体消息（带引用效果）：

```bash
curl -s -X POST 'https://open.larksuite.com/open-apis/im/v1/messages/<MESSAGE_ID>/reply' \
  -H 'Authorization: Bearer <TENANT_ACCESS_TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{
    "msg_type": "text",
    "content": "{\"text\":\"你的回复\"}"
  }'
```

`MESSAGE_ID` 从【飞书消息】中的「消息ID」字段获取。

## 完整示例：收到飞书消息后回复

当你收到如下格式的消息时：

```
【飞书消息】
发送者: ou_xxx
会话: oc_xxx
消息ID: om_xxx
内容: 用户说的话
```

回复步骤：

```bash
# 1. 获取 token（替换实际的 app_id 和 app_secret）
TOKEN=$(curl -s -X POST 'https://open.larksuite.com/open-apis/auth/v3/tenant_access_token/internal' \
  -H 'Content-Type: application/json' \
  -d '{"app_id":"<APP_ID>","app_secret":"<APP_SECRET>"}' | grep -o '"tenant_access_token":"[^"]*"' | cut -d'"' -f4)

# 2. 发送回复（使用消息中的 chat_id）
curl -s -X POST 'https://open.larksuite.com/open-apis/im/v1/messages?receive_id_type=chat_id' \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "receive_id": "oc_xxx",
    "msg_type": "text",
    "content": "{\"text\":\"你的回复内容\"}"
  }'
```

## 频率限制

- 单应用：50 次/秒
- 消息体大小不超过 150 KB

## 注意事项

- `content` 字段的值必须是 JSON 字符串（字符串里面包含 JSON），注意转义
- 如果 token 过期（返回 code 99991663），重新获取即可
- 私聊用 `open_id`（发送者ID），群聊用 `chat_id`（会话ID）
- app_id 和 app_secret 从【飞书消息】的附带信息中获取，或从会话规则中读取
