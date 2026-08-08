# dopad — Code Style Guide

## Go (`backend/`)

### Formatting
- Always run `gofmt` or `goimports` before committing
- `go vet ./...` must pass with no output

### Error handling
- Never discard errors with `_`
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- HTTP handlers log errors with `log.Printf` before responding 500; never expose internal error strings in the response body

### HTTP handlers
- CORS is applied as a middleware wrapper in `Register()` — do not call it inside individual handlers
- All JSON responses go through `writeJSON(w, statusCode, payload)` — no direct `json.NewEncoder` calls in handlers
- Use `http.StatusXxx` constants — no raw integer status codes

### Concurrency
- All shared maps must use `sync.Mutex` or `sync.RWMutex`
- Lock only the minimal critical section

### Naming
- Exported identifiers: `PascalCase`
- Unexported identifiers: `camelCase`
- No underscores in Go identifiers

### Tests
- File: `<name>_test.go` in the same package
- Use `net/http/httptest` for handler tests
- Table-driven tests preferred
- No external test frameworks — stdlib `testing` only

---

## TypeScript / React (`frontend/`)

### TypeScript
- `strict: true` is enforced in `tsconfig.json` — do not disable it
- No `any`; use `unknown` + type guards when the type is genuinely unknown
- Explicit return types on all exported functions

### React / Next.js
- App Router only — no Pages Router
- Add `"use client"` at the top of any file that uses hooks, event handlers, or browser APIs
- Private folders use the `_` prefix (`_components/`, `_lib/`) to exclude them from routing
- Server Components are the default; opt into client components only when necessary

### API calls
- All backend calls must go through `apiFetch` from `app/_lib/api.ts` — never call `fetch()` directly

### Naming
- Component files: `PascalCase.tsx` (e.g. `Login.tsx`)
- Utility/lib files: `camelCase.ts` (e.g. `api.ts`)
- Route segments: `lowercase-kebab-case`
- Named exports for all utilities and hooks; default export only for Next.js page and layout components

### Imports
- Use the `@/` path alias for all imports within the project
- Import order: React/Next.js → external packages → `@/` internal
- No `../../` relative paths for internal imports

### Styling
- Tailwind utility classes only
- Custom tokens go in `globals.css` under `@theme` — no separate config file

### Tests
- Test runner: Vitest (add to `devDependencies` when writing tests)
- Component testing: React Testing Library
- Test files: co-located as `<Component>.test.tsx`
- Test behavior from the user's perspective — not implementation details
