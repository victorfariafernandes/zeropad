# dopad

Instant online file/text sharer. Any URL path is a "pad" — visit `dopad.io/anything` to read or write content. End-to-end encrypted: the server never sees plaintext.

## Repo structure

```
backend/         Go HTTP server (port 8080)
frontend/        Next.js app (port 3000)
docs/            Project docs — read before making changes
  architecture.md
  api-spec.md
  features.md
  code-style.md
scripts/         Local dev scripts (e.g. dev-postgres.sh)
CHANGELOG.md     Updated by agents after every change (required)
.claude/commands/  Custom slash commands for Claude Code
```

## Quick start

```bash
# Backend
cd backend && go run main.go

# Frontend
cd frontend && pnpm dev
```

## Dev commands

| Task | Command |
|------|---------|
| Start local Postgres (Docker) | `eval "$(./scripts/dev-postgres.sh | tail -1)"` — exports `POSTGRES_URL` in the current shell |
| Run backend | `cd backend && go run main.go` (requires `POSTGRES_URL` + `JWT_SECRET` for auth/API-access routes) |
| Run frontend | `cd frontend && pnpm dev` |
| Backend tests | `cd backend && go test ./...` |
| Backend lint | `cd backend && go vet ./...` |
| Frontend type-check | `cd frontend && npx tsc --noEmit` |
| Frontend lint | `cd frontend && npx eslint` |
| Frontend build | `cd frontend && pnpm build` |
| Cypress E2E | `cd frontend && npx cypress run` (needs the local Postgres + backend + frontend all running) |

## Key conventions

- All backend calls go through `apiFetch` in `app/_lib/api.ts` — never call `fetch()` directly
- Auth tokens live in `sessionStorage["session_token"]` — never `localStorage`
- Every Go HTTP handler calls `cors(w, r)` first and returns if it returns `true`
- All Go JSON responses go through `writeJSON(w, statusCode, payload)`
- `"use client"` required on any React component using hooks or browser APIs
- `@/` path alias for all internal TypeScript imports — no `../../` paths
- `http.StatusXxx` constants only — no raw integers in Go

## Security

Never read, print, or include the contents of any file that may contain secrets or environment-specific values. This includes:

- `**/*.tfvars` — Terraform variable files
- `**/.env`, `**/.env.*` — environment files
- Any file named `secrets.*`, `credentials.*`, or similar

If a task requires knowing a value from one of these files, ask the user to provide the specific value directly instead of reading the file.

## Docs

Read the relevant doc before touching a feature area:

- [Architecture](docs/architecture.md) — system design, data flow, component responsibilities
- [API spec](docs/api-spec.md) — REST contract, request/response shapes, status codes
- [Features](docs/features.md) — what is implemented and the product vision
- [Code style](docs/code-style.md) — language-specific rules for Go and TypeScript/React

## Skills

| Skill | Trigger | What it does |
|-------|---------|--------------|
| `/review-docs` | "review docs", "check code against spec" | Reads `docs/` and reviews changed files for spec violations |
| `/test` | "run tests", "test everything" | Runs Go tests, frontend tests, and Cypress E2E integration tests |

## AI agent changelog rule

After making **any** change to the codebase — files added, modified, or deleted — append an entry to `CHANGELOG.md` following the format defined at the top of that file. Do this before ending the session. Do not skip this step even for small changes.
