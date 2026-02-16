# AI Hub

Web-based multi-session AI chat platform. Uses Claude Code CLI as the core agent engine, with support for any OpenAI-compatible API provider.

## Quick Start

```bash
# Build (requires Go 1.21+, Node.js 18+)
./build.sh

# Run
./ai-hub

# Options
./ai-hub --port 8080 --data ~/.ai-hub
```

Open `http://localhost:8080`

## Architecture

```
Vue3 Frontend <--WebSocket/REST--> Go Backend <--subprocess--> Claude Code CLI
                                      |
                                      +--> SQLite (~/.ai-hub/ai-hub.db)
```

Single binary deployment. Frontend is embedded via Go `embed`.

## API Reference

Base URL: `http://localhost:8080`

### Providers

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/providers` | List all providers |
| POST | `/api/v1/providers` | Create a provider |
| PUT | `/api/v1/providers/:id` | Update a provider |
| DELETE | `/api/v1/providers/:id` | Delete a provider |

### Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/sessions` | List all sessions (sorted by updated_at desc) |
| POST | `/api/v1/sessions` | Create a session |
| GET | `/api/v1/sessions/:id` | Get a session by ID |
| PUT | `/api/v1/sessions/:id` | Update a session |
| DELETE | `/api/v1/sessions/:id` | Delete a session and its messages |
| GET | `/api/v1/sessions/:id/messages` | Get all messages for a session |

### System Status

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/status` | Get dependency status (Node/npm/Claude CLI) |
| POST | `/api/v1/status/retry-install` | Retry Claude Code CLI installation |

### WebSocket

| Endpoint | Description |
|----------|-------------|
| `ws://localhost:8080/ws/chat` | Chat WebSocket. Send `{"type":"chat","session_id":0,"content":"..."}` to auto-create session, or use existing session_id. |

#### WebSocket Message Types

**Client -> Server:**
- `chat` — Send a message (session_id=0 auto-creates session)
- `stop` — Stop current streaming response

**Server -> Client:**
- `session_created` — New session created (contains session JSON)
- `chunk` — Streaming text chunk
- `done` — Response complete
- `error` — Error occurred

## Tech Stack

- Backend: Go, Gin, SQLite, gorilla/websocket
- Frontend: Vue 3, TypeScript, Vite, Pinia, vue-router
- AI Engine: Claude Code CLI (auto-installed), OpenAI-compatible API
