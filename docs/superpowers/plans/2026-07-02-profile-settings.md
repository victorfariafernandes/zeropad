# Profile Settings: account name, email, email verification

## Context

The profile page shell (`frontend/app/system/profile/ProfilePageClient.tsx`) already exists with tabs (Profile settings / API / Groups / Billing), an avatar, and a `getMe()` call, but the "Profile settings" panel content is empty. The backend already has full auth (signup/login/passkey/SIWE, JWT sessions, Postgres via `backend/adapters/db`), but the `users` table has no email-verification concept and there's no way to change your username or email after signup.

This plan wires up the "Profile settings" panel with two independently-confirmable fields — **account name** (the actual login `username`) and **email** — plus an `email_verified` flag surfaced end-to-end. Renaming the account name must reissue the session token, since JWT claims and login lookups are keyed by username. Changing the email resets verification and fires a verification email via **Resend** (account/template already created by the user; template variables `username` and `verify_url`), sent through a `Sender` interface — Resend is the only implementation, no fallback/no-op path. A verification-link landing page (public URL `/_/verify-email`, folder `frontend/app/system/verify-email/`) consumes a one-time token to flip `email_verified` to `true`.

Requires a new Go dependency: `github.com/resend/resend-go/v3` (`go get` it during implementation, in `backend/go.mod`).

## Backend

### 1. Migration — `backend/adapters/db/migrations/003_email_verification.sql`

```sql
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS email_verification_tokens (
  token      TEXT        PRIMARY KEY,
  user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email      TEXT        NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user_id
  ON email_verification_tokens (user_id);
```

Wire into `backend/adapters/db/db.go` the same way `001_users.sql`/`002_auth.sql` are embedded and applied in `migrate()`.

A separate token table (not just a column on `users`) lets a pending token remember which email it was issued for, supports invalidate-on-resend, and has a natural expiry — a stale link after a second email change is safely rejected (see `VerifyEmailToken` below).

### 2. Email sender — Resend, `backend/services/email/`

The provider is decided: **Resend**, using the account/template the user already created (template variables: `username`, `verify_url`). Interface is verification-specific (not a generic `to/subject/body` shape) so a template's variables map cleanly:

`backend/services/email/sender.go`:

```go
package email

import "context"

type Sender interface {
    SendVerificationEmail(ctx context.Context, to, username, verifyURL string) error
}
```

`backend/services/email/resend.go` — real implementation, using `github.com/resend/resend-go/v3` (new dependency — `go get github.com/resend/resend-go/v3` at implementation time). Based on the SDK's `SendEmailRequest.Template *EmailTemplate{Id, Variables map[string]any}` + `client.Emails.SendWithContext(ctx, req)`:

```go
package email

import (
    "context"
    "fmt"

    "github.com/resend/resend-go/v3"
)

type ResendSender struct {
    client     *resend.Client
    from       string
    templateID string
}

func NewResendSender(apiKey, from, templateID string) *ResendSender {
    return &ResendSender{client: resend.NewClient(apiKey), from: from, templateID: templateID}
}

func (s *ResendSender) SendVerificationEmail(ctx context.Context, to, username, verifyURL string) error {
    _, err := s.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
        From: s.from,
        To:   []string{to},
        Template: &resend.EmailTemplate{
            Id: s.templateID,
            Variables: map[string]any{
                "username":   username,
                "verify_url": verifyURL,
            },
        },
    })
    if err != nil {
        return fmt.Errorf("resend send: %w", err)
    }
    return nil
}
```

**Config (env vars, read in `main.go`, never hardcoded):** `RESEND_API_KEY`, `RESEND_FROM_EMAIL`, `RESEND_TEMPLATE_ID` — all three required, no fallback. If any is unset, `main.go` fails fast at startup the same way it already does for `JWT_SECRET` (`log.Fatal("RESEND_API_KEY env var is required")`, etc.) rather than silently degrading to a no-op sender. The actual key value is supplied by the user at deploy/runtime, never placed in code or read from a `.env`/`secrets.*` file by the agent, per `CLAUDE.md`'s security rule.

Own package (not nested under `services/auth`) since it's a cross-cutting capability.

#### Deployment wiring (GitHub Actions → Ansible → k3s)

Traced the existing chain for `JWT_SECRET` to mirror it for Resend config: `.github/workflows/deploy.yml` reads a GitHub Actions secret into the "Deploy to k3s cluster" step's `env:` block → passed to `ansible-playbook --extra-vars` → `infra/ansible/roles/k8s-manifests/tasks/main.yml` writes it into a k8s `Secret` object (`backend-secrets`) via `kubectl create secret generic ... --from-literal=` → `infra/k8s/backend/deployment.yaml` maps it into the container's env via `secretKeyRef`. Separately, non-sensitive config (`ALLOW_ORIGIN`, `OCI_BUCKET_NAME`, `OCI_NAMESPACE`) flows through a k8s `ConfigMap` (`backend-env`) consumed via `envFrom: configMapRef` in the same deployment.

Applying that same split to the three new Resend values:

- **`RESEND_API_KEY`** — sensitive, follows the `JWT_SECRET` path exactly:
  - `.github/workflows/deploy.yml` line ~149: add `RESEND_API_KEY: ${{ secrets.RESEND_API_KEY }}` to the step's `env:` block, and append `resend_api_key=$RESEND_API_KEY` to the `--extra-vars` string.
  - `infra/ansible/roles/k8s-manifests/tasks/main.yml`'s "Create or update backend secrets" task: add `--from-literal=RESEND_API_KEY={{ resend_api_key }}` alongside the existing `JWT_SECRET` literal.
  - `infra/k8s/backend/deployment.yaml`: add a `RESEND_API_KEY` entry under `env:` with `valueFrom.secretKeyRef.name: backend-secrets, key: RESEND_API_KEY`, matching the existing `JWT_SECRET` block.
- **`RESEND_FROM_EMAIL` / `RESEND_TEMPLATE_ID`** — not secret material (a sender address and a template ID), follow the `ALLOW_ORIGIN`/`OCI_NAMESPACE` ConfigMap path instead: pass through `--extra-vars` the same way, then add `--from-literal=RESEND_FROM_EMAIL={{ resend_from_email }}` and `--from-literal=RESEND_TEMPLATE_ID={{ resend_template_id }}` to the "Create or update backend ConfigMap" task. They then reach the container automatically via the deployment's existing `envFrom: configMapRef: backend-env` — no `deployment.yaml` change needed for these two.

**Flagging a wording discrepancy, not silently resolving it:** the user asked for the Resend API key to be "saved as a GitHub variable." GitHub Actions distinguishes `vars.*` (Repository/Environment **Variables** — plaintext, visible in the UI and unredacted in logs) from `secrets.*` (encrypted, redacted in logs). Every existing sensitive value in this repo's deploy pipeline (`JWT_SECRET`, `SSH_PRIVATE_KEY`, `OCI_NAMESPACE`, both Postgres backup keys) uses `secrets.*`; only genuinely non-sensitive values (`NEXT_PUBLIC_API_URL`) use `vars.*`. An API key is credential material, so this plan uses `secrets.RESEND_API_KEY` (an Actions **secret**), consistent with `JWT_SECRET`, rather than a plaintext Actions variable. The user confirmed `secrets.RESEND_API_KEY` during planning.

### 3. DB layer — `backend/adapters/db/user.go`

Add `EmailVerified bool` to the `User` struct and to the `RETURNING`/`SELECT` column lists in `CreateUser`, `GetUserByUsername`, `GetUserByWallet` (`email_verified` is `NOT NULL`, no `COALESCE` needed).

New methods, following the exact `pool.Exec`/`pool.QueryRow` + `fmt.Errorf("...: %w", err)` + typed-error pattern already used in this file:

- `GetUserByID(ctx, id) (User, bool, error)` — needed because after renaming a username, re-fetching by the *old* username no longer works; an ID-based lookup is the correct way to re-read post-mutation state (this is a small necessary addition beyond the literal request).
- `UpdateUsername(ctx, userID, newUsername) error` — `UPDATE users SET username = $1 WHERE id = $2`; maps unique-violation to `ErrDuplicateUsername` via existing `isDuplicateError`.
- `UpdateEmail(ctx, userID, newEmail string) error` — `UPDATE users SET email = $1, email_verified = false WHERE id = $2`; `newEmail == ""` writes `NULL` (empty email stays valid/allowed); maps unique-violation to `ErrDuplicateEmail`.
- `CreateEmailVerificationToken(ctx, userID, email string, ttl time.Duration) (token string, error)` — in a transaction: `DELETE FROM email_verification_tokens WHERE user_id = $1` (invalidate prior pending tokens), then `INSERT` a fresh 32-byte `crypto/rand` token (base64url, no padding) with `expires_at = now() + ttl`.
- `VerifyEmailToken(ctx, token) (userID string, err error)` — in a transaction: look up the token row, delete it (single-use regardless of outcome), reject if expired, then `UPDATE users SET email_verified = true WHERE id = $1 AND email = $2` (guarding against a stale link if the email changed again since the token was issued); `res.RowsAffected() == 0` or no matching row ⇒ `ErrInvalidToken = errors.New("invalid or expired token")`.

This is the first multi-statement/transactional operation in `user.go` — uses `d.pool.Begin(ctx)` / `tx.Commit(ctx)` / `defer tx.Rollback(ctx)` (no-op after a successful commit, standard pgx/v5 semantics already available via the existing `pgxpool` dependency).

### 4. Auth service — `backend/services/auth/service.go`

```go
type Service struct {
    db     *db.DB
    secret []byte
    mailer email.Sender
    frontendBaseURL string
}

func NewService(database *db.DB, jwtSecret []byte, mailer email.Sender, frontendBaseURL string) *Service
```

New methods:
- `UpdateUsername(ctx, userID, newUsername) (db.User, string /*fresh token*/, error)` — calls `db.UpdateUsername`, re-fetches via `GetUserByID`, mints a new token via the existing `IssueToken(s.secret, user)` (from `jwt.go`) since the old token's `Claims.Username` is now stale.
- `UpdateEmail(ctx, userID, newEmail string) (db.User, error)` — calls `db.UpdateEmail`, re-fetches via `GetUserByID`, and if `newEmail != ""` fires `sendVerificationEmail(ctx, user.ID, user.Username, newEmail)` (fire-and-forget).
- `VerifyEmail(ctx, token) error` — thin wrapper over `db.VerifyEmailToken`.
- `sendVerificationEmail(ctx, userID, username, emailAddr string)` — calls `db.CreateEmailVerificationToken` (TTL constant `emailVerificationTTL = 24 * time.Hour`), builds `verifyURL := fmt.Sprintf("%s/_/verify-email?token=%s", s.frontendBaseURL, token)` (public URL — see routing note below, **not** `/system/verify-email`), and calls `s.mailer.SendVerificationEmail(sendCtx, emailAddr, username, verifyURL)` inside a detached goroutine with its own 10s timeout context (must not use `r.Context()` — that context dies when the HTTP handler returns, before the send completes). Takes `username` as a param (not re-read from `db.User` inside the goroutine) since the caller already has it in hand from the just-fetched/just-updated user record.
- In `Signup`, after `CreateUser` succeeds, if `req.Email != ""` call `sendVerificationEmail(ctx, user.ID, user.Username, user.Email)` before returning the token — matches "on signup, if email present, send verification too."

`frontendBaseURL`: new constructor param, read from a new `FRONTEND_URL` env var in `main.go` (default to whatever `ALLOW_ORIGIN` resolves to, so no new required config in dev).

**`main.go` wiring for the mailer**, inside the existing `if database != nil` block, before constructing `authsvc.NewService(...)`. Fails fast like the existing `JWT_SECRET` check just above it — no fallback sender:

```go
resendAPIKey := os.Getenv("RESEND_API_KEY")
resendFrom := os.Getenv("RESEND_FROM_EMAIL")
resendTemplateID := os.Getenv("RESEND_TEMPLATE_ID")
if resendAPIKey == "" || resendFrom == "" || resendTemplateID == "" {
    log.Fatal("RESEND_API_KEY, RESEND_FROM_EMAIL, and RESEND_TEMPLATE_ID env vars are required")
}
mailer := email.NewResendSender(resendAPIKey, resendFrom, resendTemplateID)

frontendURL := os.Getenv("FRONTEND_URL")
if frontendURL == "" {
    frontendURL = origin // reuse the ALLOW_ORIGIN value already resolved above for CORS
}
svc := authsvc.NewService(database, jwtSecret, mailer, frontendURL)
```

No hardcoded key anywhere in code — `RESEND_API_KEY`/`RESEND_FROM_EMAIL`/`RESEND_TEMPLATE_ID` are supplied by the user as deployment secrets/env vars, never read from a `.env`/`secrets.*` file by the agent (per `CLAUDE.md`'s security rule) and never printed in this plan or logs.

### 5. HTTP handlers — new file `backend/adapters/http/profile.go`

Methods added to the existing `*AuthHandler` (already holds `svc`, `database`, `cors`, `session` — no new handler struct needed), registered in `auth.go`'s `Register(mux)`:

```go
mux.HandleFunc("PUT /auth/me/username", cors(session(h.handleUpdateUsername)))
mux.HandleFunc("PUT /auth/me/email",    cors(session(h.handleUpdateEmail)))
mux.HandleFunc("POST /auth/verify-email", cors(h.handleVerifyEmail)) // unauthenticated — link may be opened on a different device/session
```

Handler pattern matches `pad.go`/`auth.go` exactly: resolve `claims := authsvc.ClaimsFromContext(ctx)` → look up user → decode JSON body → validate → call service → `errors.Is` switch on typed errors → `writeJSON(w, http.StatusXxx, ...)`.

- `handleUpdateUsername`: body `{username}`; 400 if empty; 409 `ErrDuplicateUsername`; 200 with updated user + `token` (frontend must call `saveSession(token)`).
- `handleUpdateEmail`: body `{email}` (empty string allowed — clears email); 409 `ErrDuplicateEmail`; 200 with updated user (`email_verified: false`).
- `handleVerifyEmail`: body `{token}`; 400 if missing; 400 `ErrInvalidToken`; 200 `{ok: true}`.

Extract a shared `meResponse(user db.User, token string) map[string]any` builder (`id`, `username`, `email_verified` always; `email`/`wallet_address` only if non-empty; `token` only if non-empty) and refactor the existing `handleMe` to use it (plus `has_passkey` on top) — keeps `/auth/me` and the two new endpoints from drifting apart on response shape. `GET /auth/me` gains `email_verified` in its response as a result.

## Frontend

### 6. `frontend/app/_lib/auth.ts`

- `User.email_verified: boolean` (non-optional — backend always includes it, unlike `email`/`wallet_address`).
- `updateUsername(username: string): Promise<User & {token: string}>` — `PUT /auth/me/username` via `apiFetch`, throws on non-OK using the `{error}` body per `docs/api-spec.md` convention, calls `saveSession(data.token)` on success (existing `saveSession` helper).
- `updateEmail(email: string): Promise<User>` — `PUT /auth/me/email` via `apiFetch`.
- `verifyEmail(token: string): Promise<void>` — `POST /auth/verify-email` via `apiFetch`, no auth header (matches unauthenticated backend route).

All three follow the exact call/parse/throw shape of the existing `signup`/`loginPassword` functions in this file.

### 7. `frontend/app/system/profile/ProfileSettingsPanel.tsx` (new, `"use client"`)

Takes `user: User` and `onUserChange: (u: User) => void` props (state stays owned by `ProfilePageClient`, avoiding a duplicate fetch). Two field blocks (`UsernameField`, `EmailField`), each with its own `draft` state, dirty-check (`draft !== current value`), loading/error state, and a "Confirm change" button disabled until dirty. On confirm, call the corresponding `_lib/auth.ts` function and merge the response into `user` via `onUserChange({ ...user, ...updated })` — no refetch needed, since the PUT response already carries the authoritative post-mutation state (including `email_verified: false` after an email change). Email field shows a small "Verified" / "Not verified" pill next to it, styled with existing Tailwind conventions from `AuthPageClient.tsx`.

`ProfilePageClient.tsx` renders `{active === "profile" && <ProfileSettingsPanel user={user} onUserChange={setUser} />}` inside the currently-empty `<main>`.

### 8. New route — `frontend/app/system/verify-email/`

Per `frontend/next.config.ts` (`rewrites(): "/_/:path*" → "/system/:path*"`), everything under `app/system/` is filesystem-routed but publicly addressed via the `/_/` prefix (this is also why `backend/middlewares/reserved.go` blocks pad slugs starting with `_` — it reserves that whole namespace for these system pages). So the folder lives at `frontend/app/system/verify-email/` exactly like `system/profile` and `system/login`, but the link emailed to users — and any in-app navigation to it — must use `/_/verify-email?token=...`, not `/system/verify-email?token=...`.

- `page.tsx` — server component wrapper (matches `profile/page.tsx` pattern), renders `<VerifyEmailClient />` (check at implementation time whether Next 16 requires wrapping in `<Suspense>` for `useSearchParams()` — verify against the installed version rather than assuming).
- `VerifyEmailClient.tsx` — `"use client"`, reads `?token=` via `useSearchParams()`, calls `verifyEmail(token)` on mount, renders a verifying/success/error state.

## Docs & changelog

- `docs/api-spec.md` — add the three new endpoints (`PUT /auth/me/username`, `PUT /auth/me/email`, `POST /auth/verify-email`) and the updated `GET /auth/me` response shape (`email_verified` field). Scope this to the endpoints touched by this feature — the file currently documents zero pre-existing `/auth/*` routes at all; backfilling that gap is a separate, pre-existing-debt task, not part of this change.
- `docs/architecture.md` / `docs/features.md` — one short addition noting the profile-settings feature, the `services/email.Sender` interface (`SendVerificationEmail(ctx, to, username, verifyURL)`), and its Resend implementation (`RESEND_API_KEY`/`RESEND_FROM_EMAIL`/`RESEND_TEMPLATE_ID` env vars — all required; the backend won't start without them once the database is configured).
- `CHANGELOG.md` — one entry per repo convention, listing every file touched.

## Known trade-offs (not building mitigations, flagging only)

- Renaming the username invalidates old JWTs' `claims.Username` lookups; other open tabs/devices holding the pre-rename token will start failing `/auth/me` until they re-authenticate. This is the correct/expected consequence of "reissue token on rename," not a bug to fix here.
- **Wallet (SIWE) sign-in does not require re-signing anything on username change.** Checked `LoginWallet` in `backend/services/auth/service.go`: `verifyPersonalSign` recovers the wallet address purely from the signed message — the username is never embedded in what gets signed. `LoginWallet`'s `username` parameter is only an optional secondary cross-check performed *after* the wallet-address lookup (`if username != "" && !strings.EqualFold(user.Username, username)`), not a cryptographic binding. So a wallet user's existing signature/message stays valid forever regardless of username; renaming only reissues the session JWT (same as for password users), nothing wallet-related changes.
- No live-Postgres test harness exists (`db_test.go` only tests the no-`POSTGRES_URL` path; no CI Postgres service). New DB/service methods won't get automated table-driven tests against a real database — verification will be manual (run both dev servers with real Resend credentials, exercise signup → change username → change email → click verification link end-to-end in the browser). Go's pure-logic pieces (token generation format) can still get plain `testing` unit tests.
- Since there's no fallback sender, local dev now requires a real `RESEND_API_KEY`/`RESEND_FROM_EMAIL`/`RESEND_TEMPLATE_ID` (only when `POSTGRES_URL` is also set — the whole auth subsystem, mailer included, stays optional/off otherwise, matching the existing `if database != nil` gating for auth). There is no way to exercise signup-with-email, email-change, or verification locally without live Resend credentials.

## Verification

1. `cd backend && go vet ./... && go test ./...`
2. `cd frontend && npx tsc --noEmit && npx eslint`
3. Run both dev servers (`go run main.go`, `pnpm dev`) with `POSTGRES_URL` pointing at a local Postgres and `RESEND_API_KEY`/`RESEND_FROM_EMAIL`/`RESEND_TEMPLATE_ID` set (values supplied directly by the user, not read from any `.env` file); manually: sign up with an email → confirm a real templated email arrives with `username`/`verify_url` populated → open `/_/profile`, change account name (confirm button, verify new token persists session across reload) → change email (confirm "Not verified" pill, confirm a new email arrives) → click the verification link (or paste the token into `/_/verify-email?token=...`) → confirm "Verified" pill updates.
