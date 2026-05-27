-- +goose Up
-- loading_exceptions (#303). Tracks variance / damage / customer-change events
-- raised during container loading so that the SEAL flow can refuse to close a
-- container while disputes are still open (BR-D14). Created either manually
-- by the packer (POST /containers/:id/exceptions) or automatically by:
--   - delivery.Seal           -> SHORT_SHIPPED when actual < planned (BR-D15)
--   - delivery.AddLine / scan -> OVER_LOADED when YELLOW outcome (BR-D16, #291)
-- Resolution column captures the admin/sales decision. BACKORDER (BR-D17) and
-- SUBSTITUTE_ACCEPTED (BR-D18) drive cross-module side effects in sales.
--
-- One row per (container, sku, event) tuple. Pending = approved_by IS NULL.
-- Once approve / reject lands the row is immutable except for resolution
-- metadata.

CREATE TYPE loading_exception_type AS ENUM (
    'SHORT_SHIPPED',
    'OVER_LOADED',
    'WRONG_SKU',
    'SUBSTITUTION',
    'DAMAGED_AT_LOADING',
    'UNPLANNED_UNIT',
    'CUSTOMER_CHANGE'
);

CREATE TYPE loading_exception_resolution AS ENUM (
    'BACKORDER',
    'CANCEL_FROM_SO',
    'SUBSTITUTE_ACCEPTED',
    'WRITE_OFF',
    'DEFER_TO_NEXT'
);

CREATE TABLE loading_exceptions (
    id                      UUID                          PRIMARY KEY DEFAULT gen_random_uuid(),
    container_id            UUID                          NOT NULL REFERENCES containers(id)         ON DELETE CASCADE,
    loading_plan_id         UUID                                   REFERENCES loading_plans(id)      ON DELETE SET NULL,
    exception_type          loading_exception_type        NOT NULL,
    sku_id                  UUID                                   REFERENCES skus(id)               ON DELETE SET NULL,
    qty                     INT                                    CHECK (qty IS NULL OR qty >= 0),
    reason                  TEXT                          NOT NULL,
    photo_urls              TEXT[]                        NOT NULL DEFAULT '{}',
    approved_by             UUID                                   REFERENCES users(id)              ON DELETE SET NULL,
    approved_at             TIMESTAMPTZ,
    resolution              loading_exception_resolution,
    resolution_notes        TEXT                          NOT NULL DEFAULT '',
    carry_over_so_line_id   UUID                                   REFERENCES sales_order_lines(id)  ON DELETE SET NULL,
    substitute_sku_id       UUID                                   REFERENCES skus(id)               ON DELETE SET NULL,
    created_by              UUID                          NOT NULL REFERENCES users(id)              ON DELETE RESTRICT,
    created_at              TIMESTAMPTZ                   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_le_approval_pair CHECK (
        (approved_by IS NULL AND approved_at IS NULL)
        OR (approved_by IS NOT NULL AND approved_at IS NOT NULL)
    ),
    CONSTRAINT chk_le_backorder_link CHECK (
        (resolution = 'BACKORDER' AND carry_over_so_line_id IS NOT NULL)
        OR (resolution IS DISTINCT FROM 'BACKORDER' AND carry_over_so_line_id IS NULL)
    ),
    CONSTRAINT chk_le_substitute_link CHECK (
        (resolution = 'SUBSTITUTE_ACCEPTED' AND substitute_sku_id IS NOT NULL)
        OR (resolution IS DISTINCT FROM 'SUBSTITUTE_ACCEPTED' AND substitute_sku_id IS NULL)
    )
);

CREATE INDEX idx_le_pending
    ON loading_exceptions (container_id)
    WHERE approved_by IS NULL;

CREATE INDEX idx_le_container_created
    ON loading_exceptions (container_id, created_at DESC, id DESC);

CREATE INDEX idx_le_plan
    ON loading_exceptions (loading_plan_id)
    WHERE loading_plan_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_le_plan;
DROP INDEX IF EXISTS idx_le_container_created;
DROP INDEX IF EXISTS idx_le_pending;
DROP TABLE IF EXISTS loading_exceptions;
DROP TYPE IF EXISTS loading_exception_resolution;
DROP TYPE IF EXISTS loading_exception_type;
