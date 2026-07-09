-- 0005_session_last_seen_at.sql — track session use for owner login sessions.

ALTER TABLE sessions
    ADD COLUMN last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now();
