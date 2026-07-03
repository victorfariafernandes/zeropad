CREATE TABLE IF NOT EXISTS api_usage (
  owner_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  period_start  DATE        NOT NULL,
  request_count INTEGER     NOT NULL DEFAULT 0,
  bytes_in      BIGINT      NOT NULL DEFAULT 0,
  bytes_out     BIGINT      NOT NULL DEFAULT 0,
  PRIMARY KEY (owner_id, period_start)
);
