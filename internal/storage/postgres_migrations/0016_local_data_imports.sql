CREATE TABLE local_data_imports (
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_sha256   TEXT NOT NULL CHECK (length(source_sha256) = 64),
    source_counts   JSONB NOT NULL,
    imported_counts JSONB NOT NULL,
    completed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, source_sha256)
);
