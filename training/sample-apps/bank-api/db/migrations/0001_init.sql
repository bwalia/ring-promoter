-- 0001_init.sql — bank-api schema + seed data.
--
-- Ring Promoter training academy. This migration is intentionally tiny and
-- idempotent (IF NOT EXISTS / ON CONFLICT) so it is safe to run repeatedly —
-- whether from the Kubernetes migrate Job in the chart, from `psql` by hand, or
-- from the app itself on boot when RUN_MIGRATIONS=true.
--
-- In production you would manage migrations with a real tool (golang-migrate,
-- Flyway, Atlas, ...) and a proper version table. Kept raw SQL here so the
-- mechanics stay visible in training.

CREATE TABLE IF NOT EXISTS accounts (
    id         BIGINT PRIMARY KEY,
    owner      TEXT           NOT NULL,
    balance    NUMERIC(14, 2) NOT NULL DEFAULT 0,
    currency   TEXT           NOT NULL DEFAULT 'GBP',
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT now()
);

-- Seed row so GET /accounts/1/balance returns a real value once the DB is up.
INSERT INTO accounts (id, owner, balance, currency)
VALUES (1, 'demo', 1000.00, 'GBP')
ON CONFLICT (id) DO NOTHING;
