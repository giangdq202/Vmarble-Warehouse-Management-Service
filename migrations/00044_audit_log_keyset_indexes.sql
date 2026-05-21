-- +goose Up
-- Replace single-column indexes with composites that include
-- (created_at DESC, id DESC) so keyset pagination over (created_at, id) is a
-- pure Index Scan with no Sort node, regardless of how deep the cursor walks.
--
-- Two endpoints, two access patterns:
--   /inventory/audit-log?action=…           (action -> by-action listing)
--   /inventory/audit-log/{type}/{id}        (entity_id, entity_type -> per-entity)
--
-- Adding the id tie-breaker matters: REMNANT_BYPASSED + OVERFLOW_BYPASSED
-- can both land in the same millisecond when a planner approves a batch,
-- and a row at the page boundary would otherwise be returned twice or skipped.

DROP INDEX IF EXISTS idx_audit_action;
CREATE INDEX idx_audit_action_created_at_id
    ON inventory_audit_log (action, created_at DESC, id DESC);

DROP INDEX IF EXISTS idx_audit_entity;
CREATE INDEX idx_audit_entity_created_at_id
    ON inventory_audit_log (entity_id, entity_type, created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_entity_created_at_id;
CREATE INDEX idx_audit_entity ON inventory_audit_log (entity_id, entity_type);

DROP INDEX IF EXISTS idx_audit_action_created_at_id;
CREATE INDEX idx_audit_action ON inventory_audit_log (action);
