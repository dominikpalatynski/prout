-- +goose Up
-- +goose StatementBegin
CREATE TABLE _ping (
    id          BIGSERIAL PRIMARY KEY,
    pinged_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE _ping;
-- +goose StatementEnd
