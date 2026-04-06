-- +goose Up
-- +goose StatementBegin

-- Add is_active column; existing rows (including the admin seed from 00008)
-- default to active so no data is lost.
ALTER TABLE users ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;

-- Staging seed users.
-- Passwords are all "worker123" (bcrypt cost 12).
-- Use ON CONFLICT DO NOTHING so re-running the migration is safe.
INSERT INTO users (username, password_hash, role, is_active)
VALUES
    ('worker',  '$2a$12$zuWKDpACPyAWdSWZvvuM8eapWIqEFGWDdVqHsV2GZSZq0/99MNGGy', 'cnc',      true),
    ('worker2', '$2a$12$zuWKDpACPyAWdSWZvvuM8eapWIqEFGWDdVqHsV2GZSZq0/99MNGGy', 'cnc',      true),
    ('planner', '$2a$12$zuWKDpACPyAWdSWZvvuM8eapWIqEFGWDdVqHsV2GZSZq0/99MNGGy', 'planner',  true),
    ('warehouse', '$2a$12$zuWKDpACPyAWdSWZvvuM8eapWIqEFGWDdVqHsV2GZSZq0/99MNGGy', 'warehouse', true)
ON CONFLICT (username) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM users WHERE username IN ('worker', 'worker2', 'planner', 'warehouse');
ALTER TABLE users DROP COLUMN is_active;

-- +goose StatementEnd
