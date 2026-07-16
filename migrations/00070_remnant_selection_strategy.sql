-- +goose Up
ALTER TABLE materials
    ADD COLUMN remnant_selection_strategy TEXT NOT NULL DEFAULT 'best_fit'
        CHECK (remnant_selection_strategy IN ('best_fit', 'fifo'));

-- +goose Down
ALTER TABLE materials DROP COLUMN remnant_selection_strategy;
