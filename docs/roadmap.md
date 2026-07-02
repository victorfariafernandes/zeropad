# dopad — Monetization Roadmap

Target: freemium model with **Free** and **Paid** tiers. Paid users subscribe via crypto — no KYC, no email, no card.

## Data model

```
Account ─1:1─ Subscription ──► Billing
        ─0:N─ API Keys
        ─1:N─ User ──► Roles ──► ACL ──► Pads ──► Backup Config

Pad Bucket (OCI Object Storage, per slug):
  - content   (text blob, as today)
  - files     (binary uploads, new)
```

```
OCI Object Storage  → Pad Bucket per slug: content + files
Block Volume/SQLite → accounts, users, sessions, roles, acl, pad_meta,
                      api_keys, api_usage, namespaces, backup_configs,
                      audit_log, pad_bids
```

SQLite runs in WAL mode on the already-provisioned block volume. Migrating to PostgreSQL later is a one-adapter swap (`adapters/store/`) when scale demands it.

---

## Core principles

These constraints apply to every phase and override any conflicting implementation choice:

- **No KYC** — no payment processor, sign-up flow, or feature may require identity verification. No Stripe, no PayPal, no email.
- **Privacy-first** — the server never sees plaintext content (E2E encryption). Auth has no email requirement by design (random ID + SIWE + passkey).
- **Accepted payment methods** — ETH / ERC-20 stablecoins (USDC, DAI; natural fit with existing SIWE wallet integration), Lightning BTC, Monero.

---

## Phase 1 — Foundation (prerequisite for everything)

These are infrastructure and backend items that must exist before any tier gate can be enforced.

### 1.1 SQLite metadata store

- Mount the OCI Block Volume at `/data` via Ansible (add to `roles/volume/tasks/main.yml`)
- Open SQLite DB at `/data/dopad.db` with WAL mode (`PRAGMA journal_mode=WAL`)
- New `adapters/store/sqlite.go` implementing a `MetaStore` interface alongside the existing `OCIPadStore`
- Tables:
  - `accounts` — top-level entity; owns subscription, API keys, and users
  - `users` — belongs to an account; holds auth credentials
  - `sessions` — active session tokens
  - `roles` — named roles per account (e.g. admin, editor, viewer)
  - `acl` — `(account_id, user_id, role_id, slug)` grants; controls pad access
  - `pad_meta` — slug metadata: `owner_id`, `expires_at`, `presentation_mode`, `for_sale`, `min_bid_amount`, `min_bid_currency`
  - `namespaces` — reserved namespace → account mapping
  - `api_keys` — `sha256(key)` stored; linked to account
  - `api_usage` — per-account request counts per billing period
  - `backup_configs` — `(slug, provider, credentials_encrypted, enabled)` — per-pad backup destination
  - `audit_log` — access events for paid pads
  - `pad_bids` — marketplace bid records

### 1.2 OCI Object Storage lifecycle rule for TTL

- Add an OCI lifecycle policy in Terraform (`modules/storage/main.tf`) that auto-deletes objects tagged `tier=free` after 7 days
- On `PUT /pads/{slug}` for anonymous/free users, set the OCI object tag `tier=free` at upload time
- Paid pads are tagged `tier=paid` and never auto-deleted by the rule
- This handles most TTL cleanup without a background job; `pad_meta.expires_at` stays as a fallback index for UI queries

### 1.3 User accounts

Two supported auth methods — users can use either or link both to the same account:

**Random ID (passwordless)**
- `POST /auth/register` — client submits a chosen username; server generates a random UUID as the user identity, creates an `accounts` row and a linked `users` row, returns a session token and the UUID
- Username must be unique; UUID is the login credential — no password, no email
- `POST /auth/login` — submit the UUID, receive a new session token

**Passkey (WebAuthn)**
- `POST /auth/passkey/register/begin` + `/finish` — WebAuthn registration; stores credential public key in a `passkeys` table linked to the `users` row
- `POST /auth/passkey/login/begin` + `/finish` — WebAuthn assertion; returns a session token
- Can be used as primary login or added to an existing account as a recovery path

**SIWE / SIWX (wallet login)**
- `POST /auth/wallet` — client submits a signed SIWE message; backend verifies the signature, derives user identity from wallet address, creates or resumes an `accounts`/`users` row, returns a session token
- Aligns with the existing SIWE `KeyDeriver` already in the codebase

**Shared**
- `POST /auth/logout` — delete the `sessions` row
- Session token sent as `Authorization: Bearer <token>`; backend middleware resolves it to a `users` row and its parent `accounts` row
- Frontend: "Connect wallet" or "Generate my ID" options on the account page; token stored in `sessionStorage["session_token"]` (existing convention)

### 1.4 Pad ownership

- On `PUT /pads/{slug}`, if request is authenticated, upsert a row in `pad_meta` with `owner_id` (user) and `account_id`
- Anonymous pads: `owner_id = NULL`, always subject to free-tier limits
- `GET /pads` (new endpoint) — returns pads owned by the authenticated account

### 1.5 Tier enforcement middleware

- Go middleware reads `Authorization` header, validates session, attaches `user` + `account` + `tier` to request context
- Single `tierLimits(tier)` function returns a struct: `{ MaxSizeBytes, TTLDays, AllowCustomPaths, AllowWallet, APIQuota }`
- All limit checks go through this one function — no scattered constants

---

## Phase 2 — Free Tier Gates

Implement the constraints that define the free experience.

### 2.1 Pad TTL (7 days)

- Tag OCI objects at upload (Phase 1.2 lifecycle rule handles deletion)
- Write `expires_at = now() + 7 days` to `pad_meta` for anonymous/free users
- `GET /pads/{slug}` returns `expires_at` in the response; frontend shows an expiry countdown
- Upsell prompt: "This pad expires in X days — upgrade to keep it permanently"

### 2.2 Content size cap (100 KB)

- Reject `PUT` before forwarding to OCI if body exceeds `tierLimits.MaxSizeBytes`
- Return `413 Payload Too Large` with JSON `{ "error": "size_limit", "limit_bytes": 102400, "tier": "free" }`
- Frontend: remaining size indicator in editor toolbar; upgrade prompt on 413

### 2.3 Key derivation method restriction

- Free tier: password-based derivation only
- SIWE wallet deriver and future OAuth derivers gated behind paid tier
- Frontend: non-password methods show a locked state with a "Paid" badge

### 2.4 Write-token protected pads — Paid only

- The encrypt form (which creates a write-token-protected pad) requires an authenticated paid account
- Unencrypted public pads remain free and anonymous, no change to existing behavior

---

## Phase 3 — Paid Tier Features

### 3.1 Permanent pads

- Paid users: `expires_at = NULL` in `pad_meta`; OCI object tagged `tier=paid` (excluded from lifecycle deletion)
- Backend enforces based on resolved `tier` from middleware context

### 3.2 File uploads (up to 200 MB)

- `PUT /pads/{slug}/files/{filename}` — uploads a binary file into the Pad Bucket under a `files/` prefix in OCI
- `GET /pads/{slug}/files` — lists files attached to a pad
- `GET /pads/{slug}/files/{filename}` — retrieves a file
- `DELETE /pads/{slug}/files/{filename}` — removes a file
- Backend: use `io.LimitReader` to enforce the 200 MB cap without buffering in memory
- Return `413` with `{ "error": "file_size_limit", "limit_bytes": 209715200 }` if exceeded
- Frontend: file attachment panel in the pad editor; drag-and-drop upload

### 3.3 Audio upload

- Same file upload mechanism as 3.2; server validates MIME type is `audio/*`
- Frontend: audio player rendered inline when a pad has an attached audio file
- Rejected with `415 Unsupported Media Type` if MIME is not audio

### 3.4 ACL + Roles (zero-trust sharing)

- Account owner can define roles (admin, editor, viewer) via `POST /accounts/{id}/roles`
- Grant a role on a pad: `POST /pads/{slug}/acl { user_id, role_id }` — inserts into `acl`
- Revoke: `DELETE /pads/{slug}/acl/{user_id}`
- `GET /pads/{slug}` checks `acl` for the requesting user; returns `403` if no grant and pad is private
- Zero-trust: each grantee receives a per-user read token derived from the pad key — the server never holds the decryption key
- Frontend: sharing panel on the pad settings page; shows current grantees and their roles

### 3.5 Backup config

- `PUT /pads/{slug}/backup { provider: "s3"|"gdrive"|"dropbox", credentials_encrypted, enabled }` — stores config in `backup_configs`; credentials are encrypted client-side before sending
- A Go background job runs on a schedule (e.g. hourly), reads `backup_configs` where `enabled = true`, and pushes the current pad content + files to the configured destination
- Frontend: backup settings panel in pad settings; shows last backup time and status

### 3.6 API for CRUD pads

- `POST /api/pads/{slug}` — create or overwrite pad content (write token required for encrypted pads)
- `GET /api/pads/{slug}` — read pad content
- `DELETE /api/pads/{slug}` — delete pad (owner only)
- All endpoints authenticated via `Authorization: Bearer <api-key>`; API key must belong to a paid account
- On each request: `UPSERT api_usage ... request_count + 1`; return `429` with `X-RateLimit-Remaining` and `X-RateLimit-Reset` when quota exceeded
- `POST /api-keys` — generate a random 32-byte key, store `sha256(key)` in `api_keys`, return raw key once

### 3.7 Multiple pads per URL

- A single slug can hold multiple named pads: `/{slug}/pads/{name}` routes to a specific named slot in the Pad Bucket
- `GET/PUT /pads/{slug}/pads/{name}` — read/write a named pad within the slug's bucket
- `GET /pads/{slug}/pads` — list all named pads under a slug
- Default (unnamed) pad at `/{slug}` continues to work unchanged
- Frontend: tab or sidebar to switch between named pads within a slug

### 3.8 Billing integration (crypto, no KYC)

- `POST /billing/subscribe` — backend returns a payment address and expected amount for the chosen currency (ETH, USDC, Lightning invoice, or Monero address); linked to `accounts.id`
- A Go background watcher polls the chain / Lightning node for incoming payments; on confirmation, sets `accounts.tier` and `accounts.tier_expires_at`
- v1 simplification: payment address shown to user, admin manually confirms and flips tier; automate chain polling in v2
- No third-party webhooks, no identity verification, no stored card data
- Frontend: pricing page with crypto payment instructions, upgrade CTAs throughout the app

> **Ship 3.8 immediately after Phase 2.** You do not need all paid features before charging — start collecting revenue while the rest builds out.

### 3.9 Pad claiming and marketplace

- A paid user can claim any pad where `owner_id = NULL` (anonymous)
- `POST /pads/{slug}/claim` — sets `pad_meta.owner_id` to the claiming user
- **Listing a pad for sale**: `PUT /pads/{slug}/settings { for_sale: true, min_bid_amount, min_bid_currency }`
- **Placing a bid**: `POST /pads/{slug}/bids { amount, currency, bidder_wallet }` — inserts into `pad_bids`
- **Settling**: owner accepts via `POST /pads/{slug}/bids/{bid_id}/accept`; transfers ownership; v1 trust-based, v2 on-chain escrow

---

## Phase 4 — MCP Server

Expose pad operations to agentic clients via the Model Context Protocol.

- Implement an MCP server in Go (or as a sidecar) that wraps the existing API
- Tools exposed:
  - `read_pad(slug, name?)` — read pad content
  - `write_pad(slug, name?, content)` — write pad content
  - `list_pads(account_id)` — list pads owned by account
  - `upload_file(slug, filename, data)` — attach a file to a pad
  - `list_files(slug)` — list files attached to a pad
- Auth: API key from Phase 3.6 (`Authorization: Bearer <api-key>`)
- Each MCP tool maps 1:1 to an existing API endpoint — no new business logic in the MCP layer
- Frontend: "MCP endpoint" shown in account settings with connection instructions

---

## Phase 5 — Growth & Polish

Nice-to-haves once the core monetization loop is working.

- **Usage dashboard** — API usage, pad count, storage used, file upload quota; scoped per Account
- **Expiry notifications** — in-app countdown shown on pad page for free users; no email (no email collected)
- **Upgrade prompts** — contextual, non-intrusive; triggered on 413, TTL warnings, locked feature clicks
- **Audit log UI** — `GET /accounts/{id}/audit-log?page=&limit=`; paginated newest-first; shown in account settings
- **SQLite → PostgreSQL migration** — when write concurrency becomes a bottleneck; adapter swap in `adapters/store/`
- **Self-hosting license** — commercial license for on-prem deployments; gated behind a license key verified at startup

---

## Implementation order summary

```
Phase 1 (foundation: accounts, roles, ACL schema, auth)
        │
Phase 2 (free tier gates)
        │
Phase 3.8 (billing) ← start collecting revenue here
        │
Phase 3.1–3.7, 3.9 (paid features: files, audio, ACL, backup, API, multi-pad, marketplace)
        │
Phase 4 (MCP server)
        │
Phase 5 (growth & polish)
```
