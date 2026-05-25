-- +goose Up
-- Daily counter for container codes. Same pattern as sales_order_code_counters
-- (#289): each calendar day keeps its own monotonic sequence so the
-- human-readable code "CONT20260601-001" is dense per-day. Concurrent inserts
-- serialize on the row via INSERT … ON CONFLICT DO UPDATE … RETURNING.
CREATE TABLE container_code_counters (
    date_key  DATE PRIMARY KEY,
    last_seq  INT  NOT NULL DEFAULT 0
);

CREATE TABLE containers (
    id              UUID        PRIMARY KEY,
    code            TEXT        UNIQUE NOT NULL,
    container_type  TEXT        NOT NULL,
    -- max_cbm and max_payload_kg are container-type defaults snapshotted at
    -- create time. Stored on the row so a future container_types lookup table
    -- can be added without retro-migrating live containers.
    max_cbm         NUMERIC(8,3)  NOT NULL,
    max_payload_kg  NUMERIC(10,2) NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'OPEN',
    sealed_at       TIMESTAMPTZ,
    sealed_by       UUID        REFERENCES users(id),
    note            TEXT,
    created_by      UUID        NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_container_type   CHECK (container_type IN ('20GP','40GP','40HC')),
    CONSTRAINT chk_container_status CHECK (status IN ('OPEN','LOADING','SEALED','SHIPPED','CANCELLED')),
    CONSTRAINT chk_container_max_cbm     CHECK (max_cbm > 0),
    CONSTRAINT chk_container_max_payload CHECK (max_payload_kg > 0)
);

-- Partial index for the active set: most queries filter to "containers I can
-- still load into" (OPEN | LOADING). Sealed/shipped/cancelled rows fall out
-- of the planning UI almost immediately.
CREATE INDEX idx_containers_status_active ON containers (status)
    WHERE status IN ('OPEN','LOADING');

CREATE TABLE container_lines (
    id                    UUID        PRIMARY KEY,
    container_id          UUID        NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    sku_id                UUID        NOT NULL REFERENCES skus(id),
    qty                   INT         NOT NULL CHECK (qty > 0),
    sales_order_line_id   UUID        NOT NULL REFERENCES sales_order_lines(id),
    -- cbm_total + weight_kg_total are snapshotted at add time. Until #294
    -- adds height_mm/weight_kg to skus, the client passes them in the
    -- AddLine request body; afterwards FE auto-fills from sku.cbm * qty.
    cbm_total             NUMERIC(8,3)  NOT NULL CHECK (cbm_total >= 0),
    weight_kg_total       NUMERIC(10,2) NOT NULL CHECK (weight_kg_total >= 0),
    added_by              UUID        NOT NULL REFERENCES users(id),
    added_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_container_lines_container ON container_lines (container_id);
CREATE INDEX idx_container_lines_so_line   ON container_lines (sales_order_line_id);
CREATE INDEX idx_container_lines_sku       ON container_lines (sku_id);

-- Status transitions are recorded for audit. One row per state change with the
-- actor and an optional note (required for reopen by service-layer guard).
CREATE TABLE container_status_log (
    id            UUID        PRIMARY KEY,
    container_id  UUID        NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    from_status   TEXT,
    to_status     TEXT        NOT NULL,
    actor_id      UUID        NOT NULL REFERENCES users(id),
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_container_status_log_container
    ON container_status_log (container_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_container_status_log_container;
DROP TABLE IF EXISTS container_status_log;
DROP INDEX IF EXISTS idx_container_lines_sku;
DROP INDEX IF EXISTS idx_container_lines_so_line;
DROP INDEX IF EXISTS idx_container_lines_container;
DROP TABLE IF EXISTS container_lines;
DROP INDEX IF EXISTS idx_containers_status_active;
DROP TABLE IF EXISTS containers;
DROP TABLE IF EXISTS container_code_counters;
