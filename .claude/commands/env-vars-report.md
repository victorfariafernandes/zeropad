---
description: Scans backend and frontend code plus the deploy pipeline for every environment variable, cross-references where each is (or should be) configured, and reports a table of name/type/description/location. Flags variables that are read in code but never wired in the pipeline (the class of bug that broke the RESEND_* deploy).
allowed-tools: Read, Bash, Grep, Glob, Agent
---

# Dopad Env Var Validator

**Execution mode — always delegate, never run inline:** Do not perform Phases 1-5 yourself in the main conversation. Instead, dispatch the entire task below to a subagent via the `Agent` tool with `model: "haiku"` (subagent_type `general-purpose`). Pass this whole skill body as the agent's prompt, plus the absolute repo path. Wait for the subagent's report and relay it back to the user verbatim (or with minimal framing) — do not re-run the phases yourself after it returns.

Find every environment variable the app actually reads, then trace where each one is (or should be) configured across the deploy pipeline. Flag mismatches — a variable read in code but missing from the pipeline, or wired in the pipeline but never read, is exactly the class of bug where `secrets.X` silently resolves to an empty string.

## Phase 1 — Discover every env var read by the app

Backend (Go):
```bash
grep -rn 'os.Getenv(' backend --include="*.go" | grep -v _test.go
```

Frontend (Next.js) — only `NEXT_PUBLIC_*` vars are usable client-side; anything else in frontend code is a server-only Next.js concern:
```bash
grep -rn 'process\.env\.' frontend/app frontend/*.ts frontend/*.tsx 2>/dev/null | grep -v node_modules
```

Build a list of every distinct variable name found, with the file/line where it's read.

## Phase 2 — Trace each variable through the deploy pipeline

For each variable from Phase 1, check every layer it could be wired at:

**GitHub Actions workflows:**
```bash
grep -rn 'secrets\.\|vars\.' .github/workflows/*.yml
```
Note whether a match is `secrets.NAME` or `vars.NAME`, and whether it's inside a job with `environment: production` (or another environment) vs a job with no `environment:` key (repository-level only).

**Ansible (extra-vars → k8s):**
```bash
grep -rn '\-\-extra-vars\|from-literal' infra/ansible/**/*.yml infra/ansible/*.yml 2>/dev/null
```
Note whether each ansible var lands in a `kubectl create secret` (→ k8s Secret) or `kubectl create configmap` (→ k8s ConfigMap) command.

**Kubernetes deployment manifests:**
```bash
grep -rn 'secretKeyRef\|configMapRef\|value:' infra/k8s/**/*.yaml 2>/dev/null
```
Note whether the container actually consumes the variable via `envFrom.configMapRef`, `env[].valueFrom.secretKeyRef`, or a literal `env[].value`.

**Local dev:**
```bash
find . -maxdepth 3 -iname ".env*" -not -path "*/node_modules/*"
```
Do not print file contents (may contain secrets) — just note whether a `.env`/`.env.example` exists for local dev defaults.

## Phase 3 — Classify each variable

For each variable, determine:

- **Type** — infer from how the code uses it: compared with `!= ""` / passed to a string param → `string`; parsed with `strconv.ParseBool`/compared to `"true"` → `bool`; parsed with `strconv.Atoi`/`ParseInt` → `int`; used to build a `time.Duration` → `duration`.
- **Description** — one line, inferred from the variable name, any adjacent log message (e.g. `log.Fatal("X env var is required")`), and how it's used (e.g. passed to `NewResendSender(apiKey, ...)` → API key for the Resend email sender).
- **Location** — the *correct* place to configure it, following the precedent already established in this repo:
  - Required at runtime, sensitive (API keys, tokens, credentials) → `github actions production env secret` (mirrors `JWT_SECRET`, `RESEND_API_KEY`)
  - Required at runtime, not sensitive (IDs, addresses, non-secret config) → `github actions production env variable` if wired that way, or note if it's currently (possibly incorrectly) a secret
  - Only used for local dev, no production wiring found → `local .env only`
  - Read in code but **not found anywhere in the pipeline** → `MISSING — not wired in deploy pipeline` (this is the bug class from the RESEND_FROM_EMAIL/RESEND_TEMPLATE_ID incident: registered as the wrong kind of GitHub Actions value, so `secrets.X` resolved to empty)

## Phase 4 — Cross-check for drift

Flag two kinds of mismatches explicitly:

1. **Read in code, not wired anywhere in `.github/workflows/*.yml` and/or `infra/`** — will fail at runtime (fail-fast) or silently misbehave.
2. **Wired in the pipeline (`secrets.X`/`vars.X`, ansible extra-var, or k8s manifest) but never referenced by `os.Getenv`/`process.env` in code** — dead configuration, safe to remove or a sign the code changed without updating the pipeline.

## Phase 5 — Output the report

```
## Dopad Env Var Report

| Variable Name | Type | Description | Location |
|----------------|------|--------------|----------|
| RESEND_API_KEY | string | Resend API key for the email sender | github actions production env secret |
| RESEND_FROM_EMAIL | string | Verified sender address for Resend | github actions production env variable |
| RESEND_TEMPLATE_ID | string | Resend template ID (variables: username, verify_url) | github actions production env variable |

### Drift detected

- **Missing from pipeline:** [variable — file:line where it's read] or "None"
- **Unused in code:** [variable — where it's wired] or "None"

### Summary
X variables found, Y correctly wired, Z drift issues.
```
