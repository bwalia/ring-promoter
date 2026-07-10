-- Ring Promoter Postgres schema.
-- Applied automatically at start-up by the Postgres store (idempotent).

-- Current + previous version and last-known health for each (app, ring).
CREATE TABLE IF NOT EXISTS ring_state (
    app              TEXT        NOT NULL,
    ring             TEXT        NOT NULL,
    current_version  TEXT        NOT NULL DEFAULT '',
    previous_version TEXT        NOT NULL DEFAULT '',
    healthy          BOOLEAN     NOT NULL DEFAULT FALSE,
    -- Setting: promote onward automatically when a version lands here healthy.
    auto_promote     BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (app, ring)
);

-- Upgrade pre-existing databases created before auto_promote (idempotent).
ALTER TABLE ring_state ADD COLUMN IF NOT EXISTS auto_promote BOOLEAN NOT NULL DEFAULT FALSE;

-- User-defined application groups, shared by every user of the control plane.
-- Members are stored as a JSON array of app names.
CREATE TABLE IF NOT EXISTS app_group (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    apps       TEXT        NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
    -- Stored AI explanation of a failed entry (empty until requested).
    diagnosis    TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Upgrade pre-existing databases created before diagnosis (idempotent).
ALTER TABLE history ADD COLUMN IF NOT EXISTS diagnosis TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_history_app_created ON history (app, created_at DESC, id DESC);
