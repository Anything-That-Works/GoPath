-- +goose Up
ALTER TABLE conversations ADD COLUMN deleted_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE conversations DROP COLUMN deleted_at;