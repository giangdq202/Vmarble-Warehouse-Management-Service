-- +goose Up
-- BR-K06/K07: per-material policy for the minimum remnant size that is worth
-- keeping. When a cut would produce a remnant whose length OR width falls
-- below the threshold, the leftover is treated as waste and no remnant row is
-- inserted. A value of 0 disables enforcement (legacy materials keep the old
-- behaviour).
ALTER TABLE materials
    ADD COLUMN min_remnant_length_mm INT NOT NULL DEFAULT 0,
    ADD COLUMN min_remnant_width_mm  INT NOT NULL DEFAULT 0;

ALTER TABLE materials
    ADD CONSTRAINT materials_min_remnant_length_nonneg CHECK (min_remnant_length_mm >= 0),
    ADD CONSTRAINT materials_min_remnant_width_nonneg  CHECK (min_remnant_width_mm  >= 0);

-- +goose Down
ALTER TABLE materials
    DROP CONSTRAINT IF EXISTS materials_min_remnant_length_nonneg,
    DROP CONSTRAINT IF EXISTS materials_min_remnant_width_nonneg;

ALTER TABLE materials
    DROP COLUMN IF EXISTS min_remnant_length_mm,
    DROP COLUMN IF EXISTS min_remnant_width_mm;
