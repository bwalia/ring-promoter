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

-- User-defined dashboard topology edges and configuration-edge suppressions.
CREATE TABLE IF NOT EXISTS topology_edge (
    from_app TEXT NOT NULL,
    to_app   TEXT NOT NULL,
    PRIMARY KEY (from_app, to_app)
);

CREATE TABLE IF NOT EXISTS topology_suppression (
    from_app TEXT NOT NULL,
    to_app   TEXT NOT NULL,
    PRIMARY KEY (from_app, to_app)
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
    -- Step logs captured at failure time (kept for the newest 3 failures per
    -- app; older entries are trimmed back to '').
    logs         TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Upgrade pre-existing databases created before diagnosis/logs (idempotent).
ALTER TABLE history ADD COLUMN IF NOT EXISTS diagnosis TEXT NOT NULL DEFAULT '';
ALTER TABLE history ADD COLUMN IF NOT EXISTS logs TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_history_app_created ON history (app, created_at DESC, id DESC);

-- Operator-created ad-hoc maintenance windows. A promotion into a guarded ring
-- is allowed when now() falls inside one of these OR a config-defined recurring
-- window. An empty ring applies to every ring the app's gate guards.
CREATE TABLE IF NOT EXISTS maintenance_window (
    id         TEXT        PRIMARY KEY,
    app        TEXT        NOT NULL,
    ring       TEXT        NOT NULL DEFAULT '',
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    reason     TEXT        NOT NULL DEFAULT '',
    created_by TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_maint_app ON maintenance_window (app, ends_at DESC);

-- QA / release-engineer Go-No-Go sign-offs, one per exact (app, ring, version).
-- A promotion into a gated ring requires a stored 'go' for the version.
CREATE TABLE IF NOT EXISTS signoff (
    app        TEXT        NOT NULL,
    ring       TEXT        NOT NULL,
    version    TEXT        NOT NULL,
    decision   TEXT        NOT NULL,
    engineer   TEXT        NOT NULL DEFAULT '',
    qa_status  TEXT        NOT NULL DEFAULT '',
    note       TEXT        NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (app, ring, version)
);

-- Write-ahead journal of deploy operations in flight. A row is created just
-- before a deploy starts and deleted when the operation records its outcome;
-- a row present at start-up means a previous process died mid-operation, and
-- its outcome (ring state + history) is recovered from this record.
CREATE TABLE IF NOT EXISTS pending_op (
    id           BIGSERIAL   PRIMARY KEY,
    app          TEXT        NOT NULL,
    ring         TEXT        NOT NULL,
    action       TEXT        NOT NULL,
    from_ring    TEXT        NOT NULL DEFAULT '',
    version      TEXT        NOT NULL DEFAULT '',
    prev_version TEXT        NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Upgrade pre-existing databases created before correlation ids (idempotent).
-- The correlation id ties a history row / journaled op to every audit event of
-- the same operation.
ALTER TABLE history    ADD COLUMN IF NOT EXISTS correlation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE pending_op ADD COLUMN IF NOT EXISTS correlation_id TEXT NOT NULL DEFAULT '';

-- Append-only audit ledger: every mutating action (human, agent, or system),
-- gate verdict, and override — with who did it and which operation it belongs
-- to. Rows are only ever inserted; there is no update or delete path.
CREATE TABLE IF NOT EXISTS audit_event (
    id             BIGSERIAL   PRIMARY KEY,
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    correlation_id TEXT        NOT NULL DEFAULT '',
    actor_type     TEXT        NOT NULL DEFAULT '',
    actor          TEXT        NOT NULL DEFAULT '',
    app            TEXT        NOT NULL DEFAULT '',
    ring           TEXT        NOT NULL DEFAULT '',
    category       TEXT        NOT NULL,
    action         TEXT        NOT NULL,
    version        TEXT        NOT NULL DEFAULT '',
    detail         TEXT        NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_audit_app_id ON audit_event (app, id DESC);
