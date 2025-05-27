-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ALTER COLUMN totp_secret TYPE VARCHAR(64);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users ALTER COLUMN totp_secret TYPE VARCHAR(32);
-- +goose StatementEnd 