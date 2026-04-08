-- +goose Up
-- +goose StatementBegin

-- Seed missing role accounts for staging.
-- Passwords are bcrypt cost 12. Change immediately after first login.
--
-- accountant / acc123
-- foreman    / fore123

INSERT INTO users (username, password_hash, role)
VALUES
  (
    'accountant',
    '$2a$12$asGRqWXlyI9v7.fTIsHnY.onYlRHtyi6dWeSME4YKw8qVM344IMiG',
    'accountant'
  ),
  (
    'foreman',
    '$2a$12$Fef8UF63BbvHMo9xi0w8feHlukW3HcCWMFRvb2orhPUsXxmLKww3O',
    'foreman'
  )
ON CONFLICT (username) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM users WHERE username IN ('accountant', 'foreman');
-- +goose StatementEnd
