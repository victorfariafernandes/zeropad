# dopad — Monetization Roadmap

Target: freemium model with **Free**, **Pro** ($8/mo), and **Team** ($24/mo) tiers.

## Storage architecture

Pad content stays in **OCI Object Storage** (current setup, unchanged). All metadata — users, sessions, ownership, TTL index, API usage — lives in a **SQLite database on the existing OCI Block Volume** (`zeropad-data`). No new services or infrastructure needed.

```
OCI Object Storage   → pad content blobs (JSON per slug, unchanged)
Block Volume/SQLite  → users, sessions, pad metadata, API usage, teams
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
- Tables: `users`, `sessions`, `pad_meta`, `namespaces`, `api_keys`, `api_usage`, `teams`, `team_members`, `audit_log`, `pad_bids`
- `pad_meta` includes: `presentation_mode` (bool), `for_sale` (bool), `min_bid_amount`, `min_bid_currency` — needed for Phase 3 ownership features

### 1.2 OCI Object Storage lifecycle rule for TTL

- Add an OCI lifecycle policy in Terraform (`modules/storage/main.tf`) that auto-deletes objects tagged `tier=free` after 7 days
- On `PUT /pads/{slug}` for anonymous/free users, set the OCI object tag `tier=free` at upload time
- Pro/Team pads are tagged `tier=paid` and never auto-deleted by the rule
- This handles most TTL cleanup without a background job; `pad_meta.expires_at` stays as a fallback index for UI queries

### 1.3 User accounts

Two supported auth methods — users can use either or link both to the same account:

**Random ID (passwordless)**
- `POST /auth/register` — client submits a chosen username; server generates a random UUID as the user identity, creates a `users` row, returns a session token and the UUID
- Username must be unique; UUID is the login credential — no password, no email
- User is responsible for saving the UUID; losing it means losing account access — passkey can be added as a recovery method (see below)
- `POST /auth/login` — submit the UUID, receive a new session token
- Good for anonymous-but-persistent accounts; zero friction

**Passkey (WebAuthn)**
- `POST /auth/passkey/register/begin` + `/finish` — standard WebAuthn registration ceremony; stores the credential public key in a `passkeys` table linked to the `users` row
- `POST /auth/passkey/login/begin` + `/finish` — WebAuthn assertion; backend verifies the signature, returns a session token
- Can be used as the primary login method or added to an existing random-ID account as a recovery path
- Backend: use a WebAuthn Go library (e.g. `go-webauthn/webauthn`) for the ceremony logic
- Frontend: `navigator.credentials.create` / `navigator.credentials.get` via the WebAuthn browser API; no extra dependencies needed

**SIWE / SIWX (wallet login)**
- `POST /auth/wallet` — client submits a signed SIWE message; backend verifies the signature, derives the user identity from the wallet address, creates or resumes a `users` row, returns a session token
- Wallet address serves as the unique identifier — no password or email needed
- Aligns with the existing SIWE `KeyDeriver` already in the codebase

**Shared**
- `POST /auth/logout` — delete the `sessions` row
- Session token sent as `Authorization: Bearer <token>`; backend middleware resolves it to a `users` row
- Frontend: "Connect wallet" or "Generate my ID" options on the account page; token stored in `sessionStorage["session_token"]` (existing convention)

### 1.4 Pad ownership

- On `PUT /pads/{slug}`, if request is authenticated, upsert a row in `pad_meta` with `owner_id`
- Anonymous pads: `owner_id = NULL`, always subject to free-tier limits
- `GET /pads` (new endpoint) — returns pads owned by the authenticated user

### 1.5 Tier enforcement middleware

- Go middleware reads `Authorization` header, validates session, attaches `user` + `tier` to request context
- Single `tierLimits(tier)` function returns a struct: `{ MaxSizeBytes, TTLDays, AllowCustomPaths, AllowWallet, APIQuota }`
- All limit checks go through this one function — no scattered constants

---

## Phase 2 — Free Tier Gates

Implement the constraints that define the free experience.

### 2.1 Pad TTL (7 days)

- Tag OCI objects at upload (Phase 1.2 lifecycle rule handles deletion)
- Write `expires_at = now() + 7 days` to `pad_meta` for anonymous/free users
- `GET /pads/{slug}` returns `expires_at` in the response; frontend shows an expiry countdown
- Upsell prompt: "This pad expires in X days — upgrade to Pro for permanent pads"

### 2.2 Content size cap (100 KB)

- Reject `PUT` before forwarding to OCI if body exceeds `tierLimits.MaxSizeBytes`
- Return `413 Payload Too Large` with JSON `{ "error": "size_limit", "limit_bytes": 102400, "tier": "free" }`
- Frontend: remaining size indicator in editor toolbar; upgrade prompt on 413

### 2.3 Key derivation method restriction

- Free tier: password-based derivation only
- SIWE wallet deriver and future OAuth derivers gated behind Pro+
- Frontend: non-password methods show a locked state with a "Pro" badge

### 2.4 Write-token protected pads — Pro only

- The encrypt form (which creates a write-token-protected pad) requires an authenticated Pro+ account
- Unencrypted public pads remain free and anonymous, no change to existing behavior

---

## Phase 3 — Pro Tier Features ($8/mo)

### 3.1 Permanent pads

- Pro users: `expires_at = NULL` in `pad_meta`; OCI object tagged `tier=paid` (excluded from lifecycle deletion)
- Backend enforces based on resolved `tier` from middleware context

### 3.2 Larger size cap (10 MB)

- `tierLimits("pro").MaxSizeBytes = 10_485_760`; same middleware check as Phase 2.2

### 3.3 Pad claiming

- A paid user can claim any pad where `owner_id = NULL` (anonymous) or `owner.tier = 'free'`
- `POST /pads/{slug}/claim` — sets `pad_meta.owner_id` to the claiming user; previous owner (if any) loses write access
- Once a paid user owns a pad, it cannot be claimed — it can only be acquired via a bid (see 3.5)
- Frontend: "Claim this pad" button shown to authenticated Pro+ users on claimable pads

### 3.4 Presentation mode

- Owner toggles via `PUT /pads/{slug}/settings { presentation_mode: true }`
- In presentation mode: `GET /pads/{slug}` returns full content without requiring a write token (publicly readable); `PUT /pads/{slug}` is rejected with `403` for anyone who is not the owner
- OCI object tagged accordingly for cache-control purposes
- Frontend: read-only view with no editor, no autosave, no encrypt form; owner sees a toggle in pad settings

### 3.5 Pad marketplace & bidding

**Listing a pad for sale**
- Owner calls `PUT /pads/{slug}/settings { for_sale: true, min_bid_amount: "50", min_bid_currency: "USDC" }`
- Pad remains fully functional while listed

**Placing a bid**
- Any paid user calls `POST /pads/{slug}/bids { amount, currency, bidder_wallet }` — inserts a row into `pad_bids`
- `pad_bids` fields: `id`, `slug`, `bidder_id`, `amount`, `currency`, `status` (`pending` | `accepted` | `rejected`), `created_at`

**Settling a bid**
- Owner accepts: `POST /pads/{slug}/bids/{bid_id}/accept` — transfers `pad_meta.owner_id` to the bidder
- v1: wallet-to-wallet payment; owner's wallet address is shown to the bidder, owner confirms receipt before accepting — trust-based, no intermediary
- v2: ETH/ERC-20 escrow smart contract — bidder funds escrow on-chain, acceptance triggers ownership transfer; no fiat, no KYC at any step

### 3.6 Custom pad paths / namespaces

- `PUT /namespaces/{namespace}` — reserve a namespace for the authenticated Pro user; 409 if taken; insert into `namespaces`
- Pads at `dopad.io/{namespace}/{slug}` are routed to the owning user's quota and limits
- Frontend: namespace management page in account settings

### 3.7 SIWE wallet key derivation (unlocked)

- Already implemented in the codebase — remove the free-tier gate added in Phase 2.3

### 3.8 API access (1k req/month)

- `POST /api-keys` — generate a random 32-byte key, store `sha256(key)` in `api_keys`, return raw key once
- Requests authenticated via `Authorization: Bearer <api-key>` resolve to the owning user
- On each authenticated API request, `UPSERT api_usage ... request_count + 1`
- Return `429` with `X-RateLimit-Remaining` and `X-RateLimit-Reset` headers when quota exceeded

### 3.9 Billing integration (crypto, no KYC)

- `POST /billing/subscribe` — backend returns a payment address and expected amount for the chosen currency (ETH, USDC, Lightning invoice, or Monero address)
- A Go background watcher polls the chain / Lightning node for incoming payments; on confirmation, sets `users.tier` and `users.tier_expires_at`
- v1 simplification: payment address shown to user, admin manually confirms and flips tier; automate chain polling in v2
- No third-party webhooks, no identity verification, no stored card data
- Frontend: pricing page with crypto payment instructions, upgrade CTAs throughout the app

> **Ship 3.9 immediately after Phase 2.** You do not need all Pro features before charging — start collecting revenue while the rest builds out.

---

## Phase 4 — Team Tier Features ($24/mo flat, up to 5 seats)

### 4.1 Team workspaces

- `POST /teams` — create team, claim team slug, set owner; insert into `teams` + `team_members`
- `POST /teams/{id}/invite` — invite by username or wallet address; generates a one-time invite link; on accept, insert `team_members` row (no email required)
- Pads owned by a team (`team_id` set in `pad_meta`) are readable/writable by all members

### 4.2 Reserved namespace for team

- On team creation, reserve `dopad.io/{team-slug}/` namespace automatically (insert into `namespaces` with `team_id`)
- Shown in team settings; cannot be claimed by other users

### 4.3 OAuth key derivation (Google / Microsoft)

- Implement `GoogleKeyDeriver` and `MicrosoftKeyDeriver` in `frontend/app/_lib/crypto.ts` (same `KeyDeriver` interface)
- Key material derived from OAuth ID token — deterministic per (slug, user identity), same derivation pattern as SIWE
- Backend: `POST /auth/oauth/{provider}` to exchange code for a session; create or link `users` row by email
- Only shown to Team tier accounts in the key method picker

### 4.4 Larger size cap (100 MB)

- `tierLimits("team").MaxSizeBytes = 104_857_600`
- Use streaming body reading (`io.LimitReader`) on the backend to avoid buffering 100 MB in memory before rejecting

### 4.5 API access (20k req/month)

- Same mechanism as Phase 3.5; `tierLimits("team").APIQuota = 20_000`

### 4.6 LLM file analysis

- "Analyze with AI" button on Team pads
- Content is decrypted client-side (already in browser), then sent directly from the browser to an LLM API (e.g. OpenAI or Anthropic) — never routed through dopad's server, preserving the zero-trust model
- Frontend: analysis panel with summary / Q&A UI; user provides their own API key in team settings

### 4.7 Audit log

- Backend writes to `audit_log` table on every `GET`/`PUT` for team-owned pads
- `GET /teams/{id}/audit-log?page=&limit=` — paginated, newest first
- Frontend: audit log view in team settings

---

## Phase 5 — Growth & Polish

Nice-to-haves once the core monetization loop is working.

- **Annual billing** — 20% discount; generate annual payment addresses alongside monthly options
- **Usage dashboard** — API usage, pad count, storage used vs. tier limit
- **Expiry notifications** — in-app countdown shown on pad page for free users; no email (no email collected)
- **Upgrade prompts** — contextual, non-intrusive; triggered on 413, TTL warnings, locked feature clicks
- **SQLite → PostgreSQL migration** — when write concurrency becomes a bottleneck; adapter swap in `adapters/store/`
- **Self-hosting license** — commercial license for on-prem deployments; gated behind a license key verified at startup

---

## Implementation order summary

```
Phase 1 (SQLite + OCI lifecycle + auth)
        │
Phase 2 (free tier gates)
        │
Phase 3.9 (crypto billing)  ← start collecting revenue here
        │
Phase 3.1–3.8 (Pro features)
        │
Phase 4 (Team features)
        │
Phase 5 (growth & polish)
```
