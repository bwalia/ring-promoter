-- Ring Promoter Postgres schema.
-- Applied automatically at start-up by the Postgres store (idempotent).

-- Current + previous version and last-known health for each (app, ring).
CREATE TABLE IF NOT EXISTS ring_state (
    app              TEXT        NOT NULL,
    ring             TEXT        NOT NULL,
    current_version  TEXT        NOT NULL DEFAULT '',
    previous_version TEXT        NOT NULL DEFAULT '',
    healthy          BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (app, ring)
);

-- Append-only record of every seed / promote / rollback.
CREATE TABLE IF NOT EXISTS history (
    id           BIGSERIAL   PRIMARY KEY,
    app          TEXT        NOT NULL,
    ring         TEXT        NOT NULL,
    action       TEXT        NOT NULL,
    from_version TEXT        NOT NULL DEFAULT '',
    to_version   TEXT        NOT NULL DEFAULT '',
    result       TEXT        NOT NULL,
    message      TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_history_app_created ON history (app, created_at DESC, id DESC);
