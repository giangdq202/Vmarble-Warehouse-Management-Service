-- +goose Up
-- +goose StatementBegin
ALTER TABLE users 
    ADD COLUMN IF NOT EXISTS full_name TEXT,
    ADD COLUMN IF NOT EXISTS email TEXT UNIQUE,
    ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

-- Backfill admin user info
UPDATE users SET full_name = 'System Administrator', email = 'admin@vmarble.com' WHERE username = 'admin' AND email IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users 
    DROP COLUMN IF EXISTS full_name,
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS updated_at;
-- +goose StatementEnd
