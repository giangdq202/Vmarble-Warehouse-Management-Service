-- +goose Up
CREATE TABLE cycle_count_sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone       TEXT,
    status     TEXT NOT NULL DEFAULT 'OPEN',
    created_by UUID NOT NULL,
    posted_by  UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    posted_at  TIMESTAMPTZ
);

CREATE TABLE cycle_count_lines (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id          UUID NOT NULL REFERENCES cycle_count_sessions(id) ON DELETE CASCADE,
    entity_type         TEXT NOT NULL,
    entity_id           UUID NOT NULL,
    counted_status      TEXT NOT NULL,
    counted_location_id UUID REFERENCES storage_locations(id) ON DELETE SET NULL,
    reason              TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, entity_type, entity_id)
);

CREATE INDEX idx_cycle_count_lines_session ON cycle_count_lines (session_id);

-- +goose Down
DROP TABLE IF EXISTS cycle_count_lines;
DROP TABLE IF EXISTS cycle_count_sessions;
