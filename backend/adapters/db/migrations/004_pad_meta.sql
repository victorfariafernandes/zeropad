CREATE TABLE IF NOT EXISTS pad_meta (
  slug       TEXT        PRIMARY KEY,
  owner_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pad_meta_owner_id ON pad_meta (owner_id);
