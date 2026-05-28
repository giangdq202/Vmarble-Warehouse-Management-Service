-- +goose Up
-- BR-INV01..06: QC gate at receipt + supplier claim workflow.
-- New board_sheets statuses: PENDING_QC (gate before AVAILABLE) and REJECTED.
-- These are stored in the existing TEXT column; no DB enum to alter.

-- Backfill: any existing row with status='AVAILABLE' is treated as already
-- QC-passed (legacy). New ReceiveStock writes 'PENDING_QC'.

CREATE TABLE material_rejections (
    id                  UUID PRIMARY KEY,
    lot_id              UUID NOT NULL REFERENCES inventory_lots(id) ON DELETE RESTRICT,
    reason_code         TEXT NOT NULL CHECK (reason_code IN ('CRACK','COLOR_MISMATCH','LATE_DELIVERY','DAMAGE','WRONG_SPEC','OTHER')),
    reason_detail       TEXT,
    rejected_qty_sheets INT  NOT NULL CHECK (rejected_qty_sheets > 0),
    photo_urls          TEXT[] NOT NULL DEFAULT '{}',
    claim_amount        BIGINT,
    claim_currency      VARCHAR(3),
    claim_status        TEXT NOT NULL DEFAULT 'OPEN'
                         CHECK (claim_status IN ('OPEN','APPROVED','REJECTED','PAID')),
    resolution_notes    TEXT,
    reported_by         UUID NOT NULL REFERENCES users(id),
    reported_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_by         UUID REFERENCES users(id),
    resolved_at         TIMESTAMPTZ
);

CREATE INDEX idx_rejections_lot     ON material_rejections (lot_id);
CREATE INDEX idx_rejections_open    ON material_rejections (claim_status) WHERE claim_status = 'OPEN';
CREATE INDEX idx_rejections_keyset  ON material_rejections (reported_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_rejections_keyset;
DROP INDEX IF EXISTS idx_rejections_open;
DROP INDEX IF EXISTS idx_rejections_lot;
DROP TABLE IF EXISTS material_rejections;
