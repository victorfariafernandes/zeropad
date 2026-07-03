CREATE TABLE IF NOT EXISTS roles (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT        NOT NULL,
  can_read   BOOLEAN     NOT NULL DEFAULT true,
  can_write  BOOLEAN     NOT NULL DEFAULT false,
  can_delete BOOLEAN     NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_id, name)
);

CREATE TABLE IF NOT EXISTS acl (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slug_pattern TEXT        NOT NULL,
  role_id      UUID        NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_acl_role_id ON acl (role_id);
CREATE INDEX IF NOT EXISTS idx_acl_owner_id ON acl (owner_id);

CREATE TABLE IF NOT EXISTS api_key_roles (
  api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
  role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  PRIMARY KEY (api_key_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_api_key_roles_role_id ON api_key_roles (role_id);
