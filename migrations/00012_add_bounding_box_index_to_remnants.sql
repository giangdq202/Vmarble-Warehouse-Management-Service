-- +goose Up
-- Partial index used by FindAvailableRemnants (Best Fit search in Sprint 3).
-- Only indexes AVAILABLE rows to keep the index small; status transitions to
-- ALLOCATED / CONSUMED / WASTE remove rows from the index automatically.
CREATE INDEX idx_remnants_bounding_box
    ON remnants (bounding_box_length_mm, bounding_box_width_mm)
    WHERE status = 'AVAILABLE';

-- +goose Down
DROP INDEX IF EXISTS idx_remnants_bounding_box;
