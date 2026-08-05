# Remove sign-in/login and all account-related features

**Branch:** `feat/free-only-usage`
**Date:** 2026-08-05

## Goal

dopad reverts to a fully anonymous, free service: any pad can be created/read via URL, optionally encrypted client-side with a password or SIWE wallet signature. Every feature that depends on a logged-in account is removed — there is no partial state where some account plumbing remains "just in case."

## Rationale

Postgres, sessions/JWT, passkeys, profile settings, email verification, API keys, roles, and ACL all exist solely to support paid/authenticated accounts (`docs/architecture.md`'s "API Access" section, roadmap 3.8 billing). Pad storage itself is in-memory or OCI object storage and never touches Postgres — so removing accounts means removing Postgres entirely, not just the auth code paths on top of it.

Client-side encryption (password- and SIWE-derived AES keys, the write-token mechanism) is unrelated to accounts — it requires no server-side identity — and is unaffected.

## Scope

### Backend (Go) — delete entirely

- `services/auth/` (jwt.go, passkey.go, service.go)
- `services/apikey/` (service.go, limits.go)
- `services/role/`, `services/acl/`
- `services/email/` (sender.go, resend.go)
- `adapters/http/auth.go`, `profile.go`, `apikeys.go`, `roles.go`, `acl.go`, `api_pads.go`
- `adapters/db/` in full (db.go, user.go, acl.go, apikey.go, padmeta.go, role.go, usage.go, db_test.go) — no other subsystem uses Postgres
- `middlewares/session.go`, `middlewares/apikey.go`
- `go.mod`/`go.sum`: drop `go-webauthn`, `go-webauthn/x`, `golang-jwt/jwt`, `resend-go`, and the Postgres driver dependency

`main.go` is simplified to: select pad store (memory/OCI) → construct pad service → register pad handler → `/health` endpoint. Remove the `JWT_SECRET`, `POSTGRES_URL`, `RESEND_API_KEY`/`RESEND_FROM_EMAIL`/`RESEND_TEMPLATE_ID`, `WEBAUTHN_RP_ID`/`WEBAUTHN_RP_ORIGIN`/`WEBAUTHN_RP_NAME`, `ALLOW_ORIGIN`-adjacent auth wiring, and the fatal startup checks tied to them (keep `ALLOW_ORIGIN`/CORS — unrelated to accounts).

### Database

Add `backend/adapters/db/migrations/009_drop_accounts.sql` with `DROP TABLE` statements (in FK-safe order) for: `api_key_roles`, `acl`, `roles`, `api_keys`, `api_usage`, `pad_meta`, `email_verification_tokens`, `passkeys`, `users`. (There is no separate `sessions` table — sessions are stateless JWTs; the "sessions" mention in `architecture.md`'s directory comment refers to that concept, not a table.) This follows the existing numbered-migration convention (001–008) rather than editing prior migrations.

### Frontend (Next.js) — delete entirely

- `app/system/` in full: `login/` (AuthPageClient.tsx, page.tsx), `profile/` (ApiKeysPanel.tsx, ProfilePageClient.tsx, ProfileSettingsPanel.tsx, page.tsx), `verify-email/` (VerifyEmailClient.tsx, page.tsx)
- `app/_lib/auth.ts`, `app/_lib/apiKeys.ts`
- `app/components/UserBubble.tsx`

Update call sites that reference the above:
- `app/page.tsx` — remove `UserBubble` usage
- `app/[slug]/PadEditor.tsx` — remove `UserBubble` usage

`app/_lib/api.ts` (`apiFetch`) and `app/_lib/pads.ts` are unaffected: `apiFetch` never attached auth headers, and the pad write-token is derived client-side from the encryption key material, not from a session.

### Docs

- `docs/features.md` — remove the "Profile Settings" section; update "Core Concept" if it references accounts
- `docs/architecture.md` — remove the `auth`/`apikey`/`role`/`acl`/`email` service entries, the `db/` adapter entry, the `session`/`apikey` middleware entries, and the entire "API Access: Keys, Roles, ACL" section; trim the environment variables table to just `NEXT_PUBLIC_API_URL` (frontend) and whatever remains for the backend (e.g. `ALLOW_ORIGIN`, `OCI_BUCKET_NAME`, `OCI_NAMESPACE`)
- `docs/api-spec.md` — remove the "Authentication" and "API Access (Keys, Roles, ACL)" sections in full

### Deploy pipeline & infra

- `.github/workflows/deploy.yml` — remove `JWT_SECRET`, `RESEND_API_KEY`, `RESEND_FROM_EMAIL`, `RESEND_TEMPLATE_ID`, `POSTGRES_BACKUP_ACCESS_KEY_ID`, `POSTGRES_BACKUP_SECRET_ACCESS_KEY` from the job env and from the ansible `--extra-vars` string
- `infra/k8s/postgres/` — delete (`namespace.yaml`, `cluster.yaml`, `scheduled-backup.yaml`)
- `infra/k8s/backend/deployment.yaml` — remove the `POSTGRES_URL`, `JWT_SECRET`, `RESEND_API_KEY` env entries (and their `secretKeyRef`s)
- `infra/ansible/roles/k8s-manifests/tasks/main.yml` — remove the "Apply postgres namespace", "Create postgres backup credentials secret", "Apply postgres cluster", "Apply postgres scheduled backup" tasks (and any task reading the `zeropad-pg-app` secret URI)
- `infra/terraform/main.tf` — remove the `oci_identity_policy.postgres_backup_object_storage` resource
- `infra/terraform/modules/storage/` — remove the `postgres_backups` bucket resource and its output

These IaC files will be edited but `terraform apply` / `ansible-playbook` will **not** be run as part of this work — applying is a separate, real-infrastructure-affecting action left for the user to trigger deliberately after reviewing the diff.

### Explicitly out of scope (unchanged)

- SIWE wallet key derivation and password-based key derivation (`app/_lib/crypto.ts`)
- The write-token mechanism for encrypted pads
- Pad editor, autosave, rate limiting
- `infra/k8s/frontend/`, `infra/k8s/ingress/`, `infra/k8s/namespace.yaml`, `infra/ansible/roles/k3s-agent`, `k3s-server` (unrelated to accounts)

## Testing

- `cd backend && go build ./... && go test ./...` — confirms no dangling references to removed packages and existing pad tests still pass
- `cd backend && go vet ./...`
- `cd frontend && npx tsc --noEmit` — confirms no dangling imports of deleted modules
- `cd frontend && npx eslint`
- Manual smoke test: run backend + frontend locally, create/read/encrypt/decrypt a pad without ever hitting `/auth/*`, `/api-keys`, `/roles`, `/acl`, or `/api/pads/*` (those routes should now 404)
- Cypress E2E (`cd frontend && npx cypress run`) — any existing specs covering login/profile/API-key flows must be deleted alongside the features they test; pad-editing specs must still pass

## Non-goals

- No new anonymous rate-limiting or abuse-prevention beyond what already exists (`middlewares/ratelimit.go`, per-IP token bucket)
- No replacement mechanism for pad ownership — pads are fully public/anonymous again, matching pre-account behavior
