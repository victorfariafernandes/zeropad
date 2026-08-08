---
description: Runs Go unit tests, frontend unit tests, and Cypress E2E integration tests with both servers running. Reports a unified pass/fail summary.
allowed-tools: Bash
---

# Dopad Test Runner

Run all tests for the dopad monorepo. Run every phase regardless of failures — collect all results before reporting.

## Phase 1 — Backend Go tests

```bash
cd backend && go test ./... -v -timeout 60s 2>&1
cd backend && go vet ./... 2>&1
```

## Phase 2 — Frontend unit tests

Detect the configured test runner:

```bash
cd frontend && cat package.json | grep -E '"vitest|jest"' 2>&1
```

If **vitest** is configured:
```bash
cd frontend && npx vitest run --reporter=verbose 2>&1
```

If **jest** is configured:
```bash
cd frontend && npx jest --verbose 2>&1
```

If **neither** is found, report: "No frontend test runner configured. Add `vitest` to `devDependencies` and create `vitest.config.ts` to enable unit testing."

Always also run:
```bash
cd frontend && npx tsc --noEmit 2>&1
cd frontend && npx eslint --ext .ts,.tsx . 2>&1
```

## Phase 3 — Integration tests (Cypress E2E)

Both servers must be running. Check and start if needed.

**Backend:**
```bash
curl -s http://localhost:8080/health
```
Expect HTTP 200 with body `{"status":"ok"}`. If not, start it:
```bash
cd backend && go run main.go &
sleep 2
```

**Frontend dev server:**
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000
```
If not 200, start it:
```bash
cd frontend && pnpm dev &
sleep 5
```

**Run Cypress:**
```bash
cd frontend && cat package.json | grep cypress 2>&1
```

If Cypress is installed:
```bash
cd frontend && npx cypress run --headless 2>&1
```

If Cypress is not installed, report: "Cypress not installed. Add `cypress` to `devDependencies`, create `cypress/e2e/` tests, and add a `cypress.config.ts` to enable E2E integration testing."

Expected Cypress test files (once set up):
- `cypress/e2e/pad.cy.ts` — visit a slug, type content, reload and verify content persists
- `cypress/e2e/encryption.cy.ts` — pad written in one session decrypts correctly in another

**Stop background servers if we started them:**
```bash
kill %1 %2 2>/dev/null || true
```

## Phase 4 — Report

```
## Dopad Test Report

**Run at:** [timestamp]

### Backend

| Suite | Result | Details |
|-------|--------|---------|
| go test ./... | PASS / FAIL | [test count, failures] |
| go vet ./... | PASS / FAIL | [issues] |

### Frontend

| Check | Result | Details |
|-------|--------|---------|
| Unit tests | PASS / FAIL / NOT CONFIGURED | [runner, test count] |
| tsc --noEmit | PASS / FAIL | [error count] |
| eslint | PASS / FAIL | [error/warning count] |

### Integration (Cypress)

| Suite | Result | Details |
|-------|--------|---------|
| pad.cy.ts | PASS / FAIL / NOT CONFIGURED | |
| encryption.cy.ts | PASS / FAIL / NOT CONFIGURED | |

### Overall: PASS / FAIL

[Action items if any failures]
```
