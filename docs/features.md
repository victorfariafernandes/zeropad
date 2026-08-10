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

### Storage & Retention
- **2-day default TTL (OCI-backed deployments only)** — pads are written under a `default/` prefix in the OCI Object Storage bucket, which has a native lifecycle rule auto-deleting objects 2 days after their last-modified time, not original creation (`infra/terraform/modules/storage/main.tf`); no app-level cron or background job needed
- **Prefix-per-TTL scheme** — a future differently-retained tier would write under a new prefix (e.g. `long/`) backed by its own lifecycle rule; see `backend/adapters/store/oci.go`
- **In-memory store unaffected** — the in-memory `PadStore` (used when `OCI_BUCKET_NAME`/`OCI_NAMESPACE` are unset) has no TTL; pads live until the process restarts
- **Visible expiry countdown** — the edit screen and the "this pad is encrypted" locked screen both show a live, per-second countdown (`frontend/app/[slug]/PadTimer.tsx`) computed from the pad's last-write time (`expires_at` in the API response); shown regardless of backend for UI consistency, but only OCI-backed deployments actually enforce the deletion. Pads with no recorded write time (written before this field existed) show no countdown. Every save (including autosave) refreshes the countdown back to ~48h, matching OCI's real last-modified-based deletion clock.

---

## Differentiators (target)

- **Multiple key derivation methods** — password (done), SIWE wallet (done), OAuth Google, Microsoft (planned)
