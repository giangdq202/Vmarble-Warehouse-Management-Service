-- +goose Up
CREATE TABLE costing_adjustments (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    costing_record_id       UUID NOT NULL REFERENCES costing_records(id) ON DELETE CASCADE,
    reason                  TEXT NOT NULL,
    delta_material_amount   BIGINT NOT NULL DEFAULT 0,
    delta_material_currency TEXT NOT NULL DEFAULT 'VND',
    delta_auxiliary_amount  BIGINT NOT NULL DEFAULT 0,
    delta_auxiliary_currency TEXT NOT NULL DEFAULT 'VND',
    delta_labor_amount      BIGINT NOT NULL DEFAULT 0,
    delta_labor_currency    TEXT NOT NULL DEFAULT 'VND',
    delta_total_amount      BIGINT NOT NULL DEFAULT 0,
    delta_total_currency    TEXT NOT NULL DEFAULT 'VND',
    created_by              UUID NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_costing_adjustments_record ON costing_adjustments (costing_record_id);

-- +goose Down
DROP TABLE IF EXISTS costing_adjustments;
