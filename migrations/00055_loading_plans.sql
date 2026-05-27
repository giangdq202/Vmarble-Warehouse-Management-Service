-- +goose Up
-- loading_plans + loading_plan_lines (#301). Excel-driven container packing
-- plan: customer sends a packing-list .xlsx (one file = one container) over
-- Zalo, planner uploads, BE parses, lines become the "intent" that #291's
-- VERIFY-mode kiosk reconciles against actuals.
--
-- BR-D08 (preview only): parser fail-all — every customer_sku_code must
--   resolve via customer_sku_mappings, qty>0, unit non-empty.
-- BR-D09: depends on customer_sku_mappings (#304); enforced in service.
-- BR-D10: a fresh upload whose excel_hash matches the currently-active plan
--   for the same container is rejected (409). The partial unique index below
--   makes it impossible to land a duplicate even if the service guard races.
--   SUPERSEDED rows keep their hash so historical audit reads still work,
--   hence the WHERE clause excluding that status.

CREATE TABLE loading_plans (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    container_id    UUID        NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    excel_file_url  TEXT        NOT NULL,
    excel_hash      TEXT        NOT NULL,
    parsed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    uploaded_by     UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status          TEXT        NOT NULL CHECK (status IN ('PARSED','VALIDATED','APPROVED','SUPERSEDED')),
    version         INT         NOT NULL DEFAULT 1 CHECK (version > 0),
    notes           TEXT        NOT NULL DEFAULT '',
    approved_at     TIMESTAMPTZ,
    approved_by     UUID        REFERENCES users(id) ON DELETE SET NULL,
    superseded_at   TIMESTAMPTZ,
    superseded_by   UUID        REFERENCES loading_plans(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Active-plan lookup for GET /containers/:id/loading-plan. Partial index so
-- the historical SUPERSEDED rows do not bloat the read path.
CREATE INDEX idx_lp_container_active
    ON loading_plans(container_id)
    WHERE status <> 'SUPERSEDED';

-- BR-D10: reject re-upload of the same file while a non-superseded plan with
-- that hash exists for the container. Partial so superseded history is free
-- to keep its hash for audit.
CREATE UNIQUE INDEX uq_lp_container_hash_active
    ON loading_plans(container_id, excel_hash)
    WHERE status <> 'SUPERSEDED';

-- Latest-version lookup for the diff endpoint (GET /loading-plans/:id/diff).
CREATE INDEX idx_lp_container_version
    ON loading_plans(container_id, version DESC);

CREATE TABLE loading_plan_lines (
    id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    loading_plan_id    UUID          NOT NULL REFERENCES loading_plans(id) ON DELETE CASCADE,
    sku_id             UUID          NOT NULL REFERENCES skus(id) ON DELETE RESTRICT,
    qty_planned_pieces INT           NOT NULL CHECK (qty_planned_pieces > 0),
    unit_in_excel      TEXT          NOT NULL,
    qty_in_excel       NUMERIC(18,4) NOT NULL CHECK (qty_in_excel > 0),
    customer_sku_code  TEXT          NOT NULL,
    raw_excel_row      JSONB         NOT NULL DEFAULT '{}'::jsonb,
    excel_row_num      INT           NOT NULL CHECK (excel_row_num > 0),
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lpl_plan ON loading_plan_lines(loading_plan_id);
CREATE INDEX idx_lpl_sku  ON loading_plan_lines(sku_id);

-- +goose Down
DROP INDEX IF EXISTS idx_lpl_sku;
DROP INDEX IF EXISTS idx_lpl_plan;
DROP TABLE IF EXISTS loading_plan_lines;
DROP INDEX IF EXISTS idx_lp_container_version;
DROP INDEX IF EXISTS uq_lp_container_hash_active;
DROP INDEX IF EXISTS idx_lp_container_active;
DROP TABLE IF EXISTS loading_plans;
