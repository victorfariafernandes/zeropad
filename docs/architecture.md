# dopad — System Architecture

## System Overview

```
Browser
  │
  ▼
Next.js frontend (port 3000)
  │  All API calls via apiFetch
  ▼
Go HTTP backend (port 8080)
```

Guiding principle: **zero-trust storage**. Encryption and decryption happen on the client. The server never sees plaintext.

---

## Components

### Frontend (`frontend/`)

| Attribute | Value |
|-----------|-------|
| Framework | Next.js 16, App Router, React 19 |
| Language | TypeScript 5 (strict mode) |
| Styling | Tailwind CSS 4 (CSS-first config in `globals.css`) |
| API client | `app/_lib/api.ts` — `apiFetch` wrapper (no auth headers) |
| Package manager | pnpm |

Directory structure:
```
app/
├── _lib/
│   ├── api.ts         # apiFetch wrapper
│   ├── crypto.ts      # AES-GCM encryption + KeyDeriver abstraction
│   └── pads.ts        # getPad / setPad (calls GET+PUT /pads/{slug})
├── [slug]/
│   ├── page.tsx       # Pad page shell (server component)
│   └── PadEditor.tsx  # Pad editor (client component, auto-save, 429 handling)
├── layout.tsx         # Root layout (Geist font, dark mode)
├── page.tsx           # Home page (slug input → redirect)
└── globals.css        # Tailwind theme + CSS variables
```

### Backend (`backend/`)

| Attribute | Value |
|-----------|-------|
| Language | Go 1.21 |
| HTTP | `net/http` stdlib |
| CORS | Hardcoded to `http://localhost:3000` |
| Rate limit | 10 writes/min per IP (token bucket, `middlewares/ratelimit.go`) |

Directory structure (layered architecture):
```
backend/
├── main.go                     # Wires layers: adapters → service → HTTP router
├── services/
│   ├── pad/
│   │   └── service.go          # Business logic: pad get/set/delete, ErrNotFound sentinel
│   ├── auth/
│   │   └── service.go          # Signup/login, JWT issuance, profile updates, verification emails
│   ├── apikey/
│   │   ├── service.go          # Create/list/update/revoke keys, role attach/detach, Authenticate(rawKey)
│   │   └── limits.go           # Per-tier daily request/bandwidth quotas for /api/pads/*
│   ├── role/
│   │   └── service.go          # Create/list/update/delete named permission sets (can_read/write/delete)
│   ├── acl/
│   │   └── service.go          # Grant/revoke ACL, slug_pattern matching, Check() — the effective-permission rule
│   └── email/
│       ├── sender.go           # Sender interface: SendVerificationEmail(ctx, to, username, verifyURL)
│       └── resend.go           # Resend implementation (only implementation — no fallback)
├── adapters/
│   ├── http/
│   │   ├── pad.go              # Inward adapter: GET+PUT /pads/{slug} handlers
│   │   ├── auth.go             # Inward adapter: /auth/signup, /auth/login, /auth/me, passkey routes
│   │   ├── profile.go          # Inward adapter: PUT /auth/me/username, PUT /auth/me/email, POST /auth/verify-email
│   │   ├── apikeys.go          # Inward adapter: /api-keys, /api-keys/{id}/roles (session-authenticated)
│   │   ├── roles.go            # Inward adapter: /roles (session-authenticated)
│   │   ├── acl.go              # Inward adapter: /acl (session-authenticated)
│   │   └── api_pads.go         # Inward adapter: /api/pads/{slug} CRUD (API-key-authenticated)
│   ├── db/                     # Postgres: users (+ tier), sessions, passkeys, email verification tokens,
│   │                           # pad_meta, api_keys, roles, acl, api_key_roles, api_usage
│   └── store/
│       └── pad.go              # Outward adapter: in-memory PadStore (RWMutex); Get/Set/Delete
└── middlewares/
    ├── cors.go                 # CORS middleware wrapper (plugged via Register)
    ├── session.go               # JWT session validation, injects Claims into context
    ├── apikey.go                # API key validation, injects APIKeyClaims into context
    └── ratelimit.go            # Token-bucket per-IP rate limiter
```

### Email sending

Verification emails (on signup with an email, and on email change) go through `services/email.Sender`, an interface with a single method (`SendVerificationEmail(ctx, to, username, verifyURL)`). The only implementation is `ResendSender`, using [Resend](https://resend.com)'s Go SDK and a pre-created template (variables: `username`, `verify_url`). Configured via `RESEND_API_KEY` / `RESEND_FROM_EMAIL` / `RESEND_TEMPLATE_ID` — all required; the backend fails fast at startup if any is missing (once `POSTGRES_URL` is set and the auth subsystem is enabled). There is no no-op/fallback sender.

Layer responsibilities:
- **Services** — pure business logic, no HTTP or storage details
- **Adapters (inward)** — HTTP handlers; decode requests, call service, encode responses
- **Adapters (outward)** — implement service interfaces; currently in-memory, swappable for Redis/DB
- **Middlewares** — reusable `func(http.HandlerFunc) http.HandlerFunc` wrappers plugged at registration

---

## API Access: Keys, Roles, ACL

Lets a paying user manage pads via `/api/pads/{slug}` (see [API spec](api-spec.md)). `users.id` stands in for "account" — there's no separate `accounts` table yet (roadmap Phase 1.3 is unimplemented).

**Tables** (Postgres, `adapters/db/migrations/004`–`008`):

| Table | Purpose |
|-------|---------|
| `users.tier` | `'free'` \| `'paid'`; gates `POST /api-keys` — flipped manually until billing (roadmap 3.8) exists |
| `pad_meta` | `(slug, owner_id)` — the only place pad ownership is tracked; written by `/api/pads/*` only, never by the anonymous `/pads/{slug}` UI endpoint |
| `api_keys` | `(owner_id, name, key_hash, restricted, revoked_at)` — `key_hash` is `sha256(raw key)`; the raw key is shown once, on creation |
| `roles` | `(owner_id, name, can_read, can_write, can_delete)` — a named permission set |
| `acl` | `(owner_id, slug_pattern, role_id)` — "anyone holding `role_id` gets these permissions on pads matching `slug_pattern`" |
| `api_key_roles` | junction — which roles are attached to which key |
| `api_usage` | `(owner_id, period_start, request_count, bytes_in, bytes_out)` — daily UTC bucket for quota enforcement |

**`slug_pattern` wildcards**: an exact slug (`"notes"`) or a single trailing wildcard (`"team/eng/*"`, matching any slug with that prefix). Validated in `services/acl.ValidateSlugPattern`; matched in `services/acl.MatchesSlugPattern`.

**Effective permission check** (`services/acl.Service.Check`, used by `adapters/http/api_pads.go`):
1. Resolve the pad's owner from `pad_meta` (a `POST` on an unowned slug claims it for the requesting key's account).
2. If the pad is owned by the key's own account **and** the key isn't `restricted` → full access (the zero-setup default).
3. Otherwise, access requires an `acl` row matching the slug whose role is attached to the key (`api_key_roles`) and whose `can_<action>` is true.

**Scope notes**:
- ACL grants only apply to API keys (via attached roles) — there is no per-user grant in this design. A future, separate human-sharing flow (roadmap 3.4) can add that on top without a schema break.
- `pad_meta`/ACL only gate `/api/pads/*`. The existing `/pads/{slug}` UI endpoint is unchanged — unencrypted pads remain fully public regardless of `owner_id` (roadmap 2.4); only encrypted pads get real exclusivity, via the pre-existing write-token mechanism, which still applies independently on `/api/pads/*` too.
- `/api/pads/{slug}` also tracks bandwidth (`bytes_in`/`bytes_out`) alongside request counts, so a single large read/write is charged against the tier's daily bandwidth quota even though it's one request.

---

## Key Derivation

Pad encryption uses a `KeyDeriver` abstraction (`app/_lib/crypto.ts`). Both the AES-GCM key and the write token (used for server-side authentication) are derived from the same raw key material bytes — before those bytes are passed to `importKey`. The `CryptoKey` is held in a React ref and never leaves the browser.

```typescript
interface KeyDeriver {
  readonly id: string;
  readonly label: string;
  deriveKey(ctx: { slug: string }): Promise<CryptoKey>;
  deriveWriteToken(ctx: { slug: string }): Promise<string>;
}
```

| Method | id | Key material | AES-GCM key | Write token |
|--------|----|-------------|------------|-------------|
| Password | `"password"` | `sha3_256(password_bytes)` | `importKey(keyMaterial)` | `sha256hex(keyMaterial)` |
| Wallet (SIWE) | `"siwe"` | `sha3_256(signature_bytes)` | `importKey(keyMaterial)` | `sha256hex(keyMaterial)` |

The wallet-based method is deterministic per (slug, wallet address) pair. The `SIWEKeyDeriver` caches the raw key material promise per slug so that `deriveKey` and `deriveWriteToken` share a single `personal_sign` call — one wallet popup per unlock or encrypt operation.

Adding a new method (e.g. Google Auth) requires implementing `KeyDeriver` (both methods) and appending it to the `keyDerivers` registry in `crypto.ts`.

---

## Encryption Data Flow

### Write (encrypt / save)

```
User provides key input (password or wallet signature)
        │
        ▼
keyMaterial = sha3_256(input_bytes)   ← raw 32 bytes, computed once
        │
        ├─► importKey(keyMaterial) → CryptoKey (AES-GCM-256, extractable:false, in-memory only)
        │
        └─► sha256hex(keyMaterial) → writeToken (sent to server; AES key never leaves browser)
        │
        ▼
encryptText(key, plaintext) → base64(iv[12] || ciphertext)
makeVerifyBlob(key)         → base64(salt[16] || iv[12] || ciphertext)   ← client-side check only
        │
        ▼
PUT /pads/{slug}  { content: ciphertext, encrypted: true, verify_blob: blob,
                    write_token: writeToken [, new_write_token: newToken] }
        │
        ▼
Backend validates sha256(write_token) == stored HashedWriteToken
        │
        ▼
Backend stores opaque ciphertext + sha256(write_token) — never decrypts
```

### Read (unlock)

```
User provides key input (password or wallet signature)
        │
        ▼
keyMaterial = sha3_256(input_bytes)
        │
        ├─► CryptoKey  (for decryption)
        └─► writeToken (sha256hex(keyMaterial))
        │
        ▼
GET /pads/{slug}  X-Write-Token: writeToken
        │         (unauthenticated GET returns only slug/encrypted/deriver_id)
        ▼
Backend validates sha256(writeToken) == HashedWriteToken → returns ciphertext + verify_blob
        │
        ▼
checkVerifyBlob(key, verify_blob)  ← client-side key confirmation
        │
        ▼
decryptText(key, ciphertext) → plaintext  (never sent to server)
```

---

## Environment Variables

| Variable | Service | Default |
|----------|---------|---------|
| `NEXT_PUBLIC_API_URL` | Frontend | `http://localhost:8080` |
