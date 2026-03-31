-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Default admin user: password = "admin123" (bcrypt cost 12)
-- Change immediately in production via PATCH /api/auth/me or direct DB update.
INSERT INTO users (username, password_hash, role)
VALUES (
    'admin',
    '$2a$12$iDdgiMx3dqklZKb2Uo3LVu9H32RGZeXNwmAKY6HvOwL0xfQbaYrA2',
    'admin'
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
