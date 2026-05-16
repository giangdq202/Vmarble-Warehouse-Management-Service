-- +goose Up
-- Sprint 6 #249: allow planners to cancel APPROVED plans with audit trail.
-- Adds reason/actor/timestamp columns so a CANCELED plan retains the
-- explanation and the user who issued the cancel. All three columns are
-- nullable because pre-existing CANCELED rows did not capture this data.
ALTER TABLE production_plans
    ADD COLUMN canceled_reason TEXT,
    ADD COLUMN canceled_at     TIMESTAMPTZ,
    ADD COLUMN canceled_by     UUID;

-- +goose Down
ALTER TABLE production_plans
    DROP COLUMN IF EXISTS canceled_by,
    DROP COLUMN IF EXISTS canceled_at,
    DROP COLUMN IF EXISTS canceled_reason;
