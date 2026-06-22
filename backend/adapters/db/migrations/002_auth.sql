ALTER TABLE users
  ADD COLUMN IF NOT EXISTS username       TEXT UNIQUE NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS email          TEXT UNIQUE,
  ADD COLUMN IF NOT EXISTS password_hash  TEXT,
  ADD COLUMN IF NOT EXISTS wallet_address TEXT UNIQUE;

ALTER TABLE users ALTER COLUMN username DROP DEFAULT;

CREATE TABLE IF NOT EXISTS passkeys (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_id BYTEA       UNIQUE NOT NULL,
  public_key    BYTEA       NOT NULL,
  sign_count    BIGINT      NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
