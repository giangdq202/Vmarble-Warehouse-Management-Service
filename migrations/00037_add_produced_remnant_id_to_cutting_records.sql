-- +goose Up
-- +goose StatementBegin

-- Adds an explicit link from each cutting record to the remnant it produced
-- (if any), so the cutting kiosk can render combined WIP + remnant labels
-- without resorting to fragile time-proximity joins.
ALTER TABLE cutting_records
    ADD COLUMN IF NOT EXISTS produced_remnant_id UUID REFERENCES remnants(id) ON DELETE SET NULL;

-- Backfill existing rows. Each historical cut produced at most one remnant
-- whose parent linkage matches the cut's source. We pick the remnant with
-- the closest created_at to the cut (within 5 seconds) to disambiguate
-- successive cuts on the same parent.
UPDATE cutting_records cr
SET produced_remnant_id = sub.remnant_id
FROM (
    SELECT DISTINCT ON (cr.id)
        cr.id AS cut_id,
        r.id  AS remnant_id
    FROM cutting_records cr
    JOIN remnants r
      ON (
           (cr.sheet_id           IS NOT NULL AND r.parent_board_id   = cr.sheet_id           AND r.parent_remnant_id IS NULL)
        OR (cr.remnant_source_id  IS NOT NULL AND r.parent_remnant_id = cr.remnant_source_id)
      )
     AND ABS(EXTRACT(EPOCH FROM (r.created_at - cr.created_at))) < 5
    ORDER BY cr.id, ABS(EXTRACT(EPOCH FROM (r.created_at - cr.created_at)))
) AS sub
WHERE cr.id = sub.cut_id
  AND cr.produced_remnant_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_cutting_records_produced_remnant
    ON cutting_records (produced_remnant_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_cutting_records_produced_remnant;
ALTER TABLE cutting_records DROP COLUMN IF EXISTS produced_remnant_id;
-- +goose StatementEnd
