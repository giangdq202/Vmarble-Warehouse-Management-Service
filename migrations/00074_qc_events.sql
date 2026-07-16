-- +goose Up

-- QC result column on work_orders for quick status reads without joining qc_events.
ALTER TABLE work_orders
    ADD COLUMN qc_status TEXT CHECK (qc_status IN ('QC_PASSED', 'QC_FAILED'));

-- qc_events records every QC checkpoint scan (QC_PASSED or QC_FAILED) linked
-- back to the originating barcode scan_event so the full audit trail is traceable.
CREATE TABLE qc_events (
    id            UUID PRIMARY KEY,
    work_order_id UUID NOT NULL REFERENCES work_orders(id),
    barcode_id    UUID NOT NULL REFERENCES barcodes(id),
    scan_event_id UUID NOT NULL REFERENCES scan_events(id),
    result        TEXT NOT NULL CHECK (result IN ('QC_PASSED', 'QC_FAILED')),
    scanned_by    UUID NOT NULL,
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_qc_events_work_order_id ON qc_events (work_order_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_qc_events_work_order_id;
DROP TABLE IF EXISTS qc_events;

ALTER TABLE work_orders
    DROP COLUMN IF EXISTS qc_status;
