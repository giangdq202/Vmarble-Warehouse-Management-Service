-- +goose Up
CREATE TABLE wo_blockers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_order_id UUID NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    reason        TEXT NOT NULL CHECK (reason IN ('MATERIAL_DELAYED','MATERIAL_REJECTED','MACHINE_DOWN','OTHER')),
    detail        TEXT NOT NULL DEFAULT '',
    created_by    UUID NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_by   UUID,
    resolved_at   TIMESTAMPTZ
);

CREATE INDEX idx_wo_blockers_wo_open ON wo_blockers (work_order_id) WHERE resolved_at IS NULL;
COMMENT ON TABLE wo_blockers IS 'Tracks blocking reasons that prevent a WO from advancing status';

-- +goose Down
DROP TABLE IF EXISTS wo_blockers;
