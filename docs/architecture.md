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
│   └── pad/
│       └── service.go          # Business logic: pad get/set/delete, ErrNotFound sentinel
├── adapters/
│   ├── http/
│   │   └── pad.go              # Inward adapter: GET+PUT /pads/{slug} handlers
│   └── store/
│       └── pad.go              # Outward adapter: in-memory PadStore (RWMutex); Get/Set/Delete
└── middlewares/
    ├── cors.go                 # CORS middleware wrapper (plugged via Register)
    └── ratelimit.go            # Token-bucket per-IP rate limiter
```

Layer responsibilities:
- **Services** — pure business logic, no HTTP or storage details
- **Adapters (inward)** — HTTP handlers; decode requests, call service, encode responses
- **Adapters (outward)** — implement service interfaces; currently in-memory, swappable for Redis/DB
- **Middlewares** — reusable `func(http.HandlerFunc) http.HandlerFunc` wrappers plugged at registration

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
| `ALLOW_ORIGIN` | Backend | `http://localhost:3000` — CORS allowed origin |
| `OCI_BUCKET_NAME` | Backend | unset — with `OCI_NAMESPACE`, selects the OCI Object Storage pad store; otherwise falls back to in-memory |
| `OCI_NAMESPACE` | Backend | unset — see `OCI_BUCKET_NAME` |
