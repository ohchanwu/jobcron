-- 0014_user_ai_credentials.sql — encrypted per-user provider credentials.

CREATE TABLE user_ai_credentials (
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL,
    ciphertext          BYTEA NOT NULL,
    nonce               BYTEA NOT NULL,
    encryption_version  SMALLINT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, provider),
    CHECK (provider <> ''),
    CHECK (octet_length(ciphertext) > 16),
    CHECK (octet_length(nonce) = 12),
    CHECK (encryption_version > 0)
);
