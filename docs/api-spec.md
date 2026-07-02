# dopad — REST API Specification

## Base URL

| Environment | URL |
|-------------|-----|
| Development | `http://localhost:8080` |
| Production | `https://api.dopad.io` |

## General Conventions

- All responses: `Content-Type: application/json`
- Errors always return `{ "error": "<message>" }`
- CORS: allowed origin `http://localhost:3000`, methods `GET PUT OPTIONS`, header `Content-Type`

---

## Endpoints

### `GET /pads/{slug}`

Returns the content of a pad by slug.

**Request headers (optional):**

| Header | Description |
|--------|-------------|
| `X-Write-Token` | Write token derived from the encryption key. Required to retrieve content of an authenticated encrypted pad. |

**Response 200 — unencrypted pad or legacy encrypted pad (no write token set):**
```json
{ "slug": "my-page", "content": "hello world", "encrypted": false, "verify_blob": "", "deriver_id": "" }
```

**Response 200 — encrypted pad, no `X-Write-Token` header (metadata only):**
```json
{ "slug": "my-page", "encrypted": true, "deriver_id": "password" }
```

**Response 200 — encrypted pad, valid `X-Write-Token` (full response):**
```json
{ "slug": "my-page", "content": "<base64(iv||ciphertext)>", "encrypted": true, "verify_blob": "<base64(salt||iv||ciphertext)>", "deriver_id": "password" }
```

**Response 400:**
```json
{ "error": "missing slug" }
```

**Response 403:**
```json
{ "error": "invalid write token" }
```

**Response 404:**
```json
{ "error": "pad not found" }
```

---

### `PUT /pads/{slug}`

Creates or overwrites a pad. Rate-limited to **10 requests/minute per IP**.

**Body:**
```json
{ "content": "hello world", "encrypted": false, "verify_blob": "", "deriver_id": "", "write_token": "", "new_write_token": "" }
```

For encrypted pads (first encryption):
```json
{
  "content": "<base64(iv || ciphertext)>",
  "encrypted": true,
  "verify_blob": "<base64(salt || iv || ciphertext)>",
  "deriver_id": "password",
  "write_token": "<sha256hex(key_material)>",
  "new_write_token": ""
}
```

For encrypted pads (key change — re-encryption with a new key):
```json
{
  "content": "<base64(iv || ciphertext)>",
  "encrypted": true,
  "verify_blob": "<base64(salt || iv || ciphertext)>",
  "deriver_id": "password",
  "write_token": "<old key's sha256hex(key_material)>",
  "new_write_token": "<new key's sha256hex(key_material)>"
}
```

| Field | Description |
|-------|-------------|
| `write_token` | Required when the existing pad is encrypted. Must equal the token set during first encryption. |
| `new_write_token` | Provided only when changing the encryption key. Replaces the stored token after validating `write_token`. |

**Response 200:**
```json
{ "slug": "my-page", "content": "...", "encrypted": true, "verify_blob": "...", "deriver_id": "password" }
```

**Response 400:**
```json
{ "error": "missing slug" }
{ "error": "invalid body" }
```

**Response 403:**
```json
{ "error": "write token required" }
{ "error": "invalid write token" }
```

**Response 429:**
```json
{ "error": "rate limit exceeded" }
```

---

## Authentication

The following endpoints are documented as of the profile-settings feature; other pre-existing `/auth/*` endpoints (signup, login, logout, passkey) are not yet documented here.

### `GET /auth/me`

Returns the authenticated user. Requires `Authorization: Bearer <session_token>`.

**Response 200:**
```json
{ "id": "...", "username": "my-page", "email": "me@example.com", "email_verified": false, "wallet_address": "0x...", "has_passkey": false }
```
`email` and `wallet_address` are omitted when unset. `email_verified` is always present.

**Response 401:**
```json
{ "error": "missing or invalid token" }
```

### `PUT /auth/me/username`

Renames the authenticated user's account name (login `username`). Requires `Authorization: Bearer <session_token>`.

**Body:**
```json
{ "username": "new-name" }
```

**Response 200** — same shape as `GET /auth/me`, plus a fresh session token (the old token's claims reference the previous username and stop resolving):
```json
{ "id": "...", "username": "new-name", "email_verified": false, "has_passkey": false, "token": "<new session token>" }
```

**Response 400:**
```json
{ "error": "username is required" }
```

**Response 409:**
```json
{ "error": "username already taken" }
```

### `PUT /auth/me/email`

Sets the authenticated user's email. Resets `email_verified` to `false` and triggers a new verification email. Requires `Authorization: Bearer <session_token>`.

**Body:**
```json
{ "email": "me@example.com" }
```
`email` may be an empty string to clear it — no email is a valid state.

**Response 200** — same shape as `GET /auth/me`:
```json
{ "id": "...", "username": "my-page", "email": "me@example.com", "email_verified": false, "has_passkey": false }
```

**Response 409:**
```json
{ "error": "email already registered" }
```

### `POST /auth/verify-email`

Consumes a one-time email verification token (sent via the link in the verification email). Unauthenticated — the link may be opened on a different device/session than the one that requested it.

**Body:**
```json
{ "token": "<token from the verification email link>" }
```

**Response 200:**
```json
{ "ok": true }
```

**Response 400:**
```json
{ "error": "token is required" }
{ "error": "invalid or expired token" }
```
