-- +goose Up
-- container_lines_history (#302). Strict re-load policy: when an Excel v2
-- supersedes v1 on a container that already has scanned lines, the service
-- snapshots every container_lines row here, then DELETEs them so packers
-- restart from zero. The v1 plan stays in loading_plans as SUPERSEDED for
-- audit; this table is the line-level audit trail.
--
-- BR-D11: every supersede must produce one row per dropped container_line in
--   the same transaction that DELETEs from container_lines and flips the
--   plans' status. raw_snapshot freezes the entire row in case a barcode/SKU
--   id is later recycled or a foreign-key target gets deleted.
-- BR-D12: SEALED containers refuse supersede unless an admin force-unseals
--   first; that guard lives in the service layer.

CREATE TABLE container_lines_history (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    original_line_id    UUID        NOT NULL,
    container_id        UUID        NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    sku_id              UUID        NOT NULL,
    barcode_id          UUID,
    superseded_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    superseded_by_plan  UUID        NOT NULL REFERENCES loading_plans(id) ON DELETE RESTRICT,
    superseded_by_user  UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason              TEXT        NOT NULL DEFAULT 'PLAN_V2_SUPERSEDE',
    raw_snapshot        JSONB       NOT NULL
);

-- Drives GET /containers/:id/lines-history?plan_id=... and the audit
-- timeline. DESC matches the FE rendering (newest supersede on top).
CREATE INDEX idx_clh_container ON container_lines_history(container_id, superseded_at DESC);

-- Filter by plan when one supersede event needs to be re-listed for a
-- worker explanation. Plan ids land in this table only when status flipped
-- to SUPERSEDED, so the column is never null.
CREATE INDEX idx_clh_plan ON container_lines_history(superseded_by_plan);

-- +goose Down
DROP INDEX IF EXISTS idx_clh_plan;
DROP INDEX IF EXISTS idx_clh_container;
DROP TABLE IF EXISTS container_lines_history;
