# Repository Guidelines

## Project Structure & Module Organization
- `main.go`: application entrypoint; embeds frontend and built-in assets.
- `server/`: Go backend.
- `server/api/`: HTTP/WebSocket handlers.
- `server/core/`: runtime logic (Claude process pool, vector watcher/engine integration, templates).
- `server/store/`: SQLite data access models and persistence.
- `web/`: Vue 3 + TypeScript frontend (`src/views`, `src/components`, `src/stores`, `src/composables`).
- `vector-engine/`: Python vector service scripts used by backend features.
- `skills/`, `claude/`: embedded built-in skill/rule assets.
- Build output: `dist/` and `web/dist/`.

## Build, Test, and Development Commands
- `make` or `make all`: build frontend and current-platform binary.
- `make frontend`: run `web` production build only.
- `make build`: compile Go binary into `dist/` (expects built frontend).
- `make release`: cross-compile macOS/Linux/Windows artifacts.
- `make clean`: remove build artifacts.
- `cd web && npm run dev`: start frontend dev server (Vite).
- `go run . --port 8080`: run backend locally.

## Coding Style & Naming Conventions
- Go: always `gofmt`-formatted (tabs, idiomatic Go naming, exported identifiers in PascalCase).
- Vue/TS: existing files use 2-space indentation and `PascalCase.vue` component names (for example, `ChatPanel.vue`).
- Prefer clear package/module boundaries (`api`, `core`, `store`) over large mixed files.
- Keep API route/resource naming consistent with current REST patterns under `/api/v1`.

## Testing Guidelines
- No automated test files are currently committed (`go test ./...` reports “no test files”).
- Before opening a PR, run smoke checks:
- `go test ./...` (compilation/regression check for Go packages).
- `cd web && npm run build` (type-check + production frontend build).
- Add tests with any non-trivial logic changes:
- Go tests as `*_test.go` alongside target packages.
- Frontend tests in `web/src` with `*.spec.ts` naming.

## Commit & Pull Request Guidelines
- Follow Conventional Commits seen in history: `fix(scope): ...`, `feat(scope): ...`, `docs: ...`.
- Keep subject lines imperative and scoped (example: `fix(vector): wait for bootstrap before search`).
- Reference issues in commit or PR body (pattern used: `closes #123`).
- PRs should include:
- concise problem/solution summary,
- impacted modules (for example, `server/core`, `web/src/views`),
- verification steps/commands run,
- screenshots for UI-visible changes.
