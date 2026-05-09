-- +goose Up
ALTER TABLE inventory_audit_log
    ADD COLUMN IF NOT EXISTS metadata JSONB;

CREATE INDEX IF NOT EXISTS idx_audit_action ON inventory_audit_log (action);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_action;

ALTER TABLE inventory_audit_log
    DROP COLUMN IF EXISTS metadata;
