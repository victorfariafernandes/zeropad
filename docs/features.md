# dopad — Features

## Core Concept

dopad is an online instant file sharer. Any URL path is a "pad" — visit `dopad.io/anything` to read or write content. No login required. Inspired by dontpad.com.

---

## Implemented Features

### End-to-End Encryption
- **AES-GCM-256** — content encrypted in browser before upload; server stores ciphertext only
- **Password-based key** — SHA3-256(password) → AES key; password never sent to server
- **Wallet-based key (SIWE)** — `personal_sign` of deterministic per-pad message → SHA3-256(signature) → AES key; re-derivable with the same wallet anytime
- **Verify blob** — encrypted sentinel stored alongside ciphertext; used client-side only to confirm the correct key was entered before attempting decryption
- **Extensible `KeyDeriver` interface** — new methods (Google Auth, Microsoft, etc.) added by implementing the interface and registering in `keyDerivers`

### Write-Token Authentication for Encrypted Pads
- **Cryptographic write token** — derived from the same key material as the AES-GCM key via `sha256hex(keyMaterial)`; the AES-GCM key itself never leaves the browser
- **Server stores only a hash** — `sha256(write_token)` stored in the pad record; the raw token is never persisted or returned
- **Two-phase GET** — unauthenticated `GET /pads/{slug}` returns only `{ slug, encrypted, deriver_id }` for protected pads; sending `X-Write-Token` with the correct token returns the full ciphertext and verify blob
- **Authenticated PUT** — `PUT /pads/{slug}` on an existing encrypted pad requires `write_token` in the body; 403 returned on missing or wrong token
- **Key change flow** — re-encrypting with a new key sends `write_token` (old, for validation) and `new_write_token` (new, to replace the stored hash) in a single PUT
- **Unencrypted pads unaffected** — no token required; fully public as before
- **Backward-compatible migration** — pre-existing encrypted pads have no stored token; they remain accessible until the next write, which locks them going forward

### Pad Editor
- Auto-save with 800 ms debounce
- Rate-limit feedback (429 → amber warning in header)
- Method picker UI on lock screen and encrypt form
- Key and write token held in React refs; autosave re-encrypts on every edit

---

## Differentiators (target)

- **Multiple key derivation methods** — password (done), SIWE wallet (done), OAuth Google, Microsoft (planned)
- **Premium tier** — unlimited TTL, larger file size limits, local LLM file analysis
