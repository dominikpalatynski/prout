-- +goose Up
-- +goose StatementBegin
CREATE TABLE operation_request_history_entries (
    id                   BIGSERIAL PRIMARY KEY,
    operation_request_id BIGINT NOT NULL REFERENCES operation_requests(id) ON DELETE CASCADE,
    kind                 TEXT NOT NULL CHECK (kind IN (
        'request_started',
        'request_retried',
        'step_entered',
        'step_completed',
        'step_failed',
        'request_completed',
        'request_failed'
    )),
    step                 TEXT,
    message              TEXT NOT NULL,
    details_json         JSONB,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX operation_request_history_entries_request_id_idx
    ON operation_request_history_entries (operation_request_id, id ASC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE operation_request_history_entries;
-- +goose StatementEnd
