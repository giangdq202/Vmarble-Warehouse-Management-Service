-- +goose Up
CREATE TABLE allocations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fg_pool_id          UUID NOT NULL UNIQUE REFERENCES fg_pool(id) ON DELETE CASCADE,
    sales_order_line_id UUID NOT NULL,
    allocation_type     TEXT NOT NULL CHECK (allocation_type IN ('hard', 'soft')),
    released_by         UUID,
    released_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_allocations_sol  ON allocations (sales_order_line_id);
CREATE INDEX idx_allocations_type ON allocations (allocation_type);

-- Backfill HARD allocations from existing fg_pool rows that already carry a SOL.
INSERT INTO allocations (id, fg_pool_id, sales_order_line_id, allocation_type, created_at)
SELECT gen_random_uuid(), id, sales_order_line_id, 'hard', created_at
FROM   fg_pool
WHERE  sales_order_line_id IS NOT NULL
  AND  status NOT IN ('DISPOSED');

-- +goose Down
DROP TABLE IF EXISTS allocations;
