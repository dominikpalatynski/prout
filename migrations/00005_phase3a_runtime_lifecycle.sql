-- +goose Up
-- +goose StatementBegin
CREATE TABLE pull_requests (
    id                      BIGSERIAL PRIMARY KEY,
    repository_id           BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    pr_number               BIGINT NOT NULL,
    github_pull_request_id  BIGINT UNIQUE,
    current_head_commit_sha TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pull_requests_repository_pr_number_unique
        UNIQUE (repository_id, pr_number)
);

CREATE TABLE runtime_environments (
    id                      BIGSERIAL PRIMARY KEY,
    repository_id           BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    pull_request_id         BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    type                    TEXT NOT NULL,
    status                  TEXT NOT NULL CHECK (status IN ('preparing', 'prepared', 'failed', 'superseded')),
    target_pr_head_commit_sha TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE operation_requests (
    id                                  BIGSERIAL PRIMARY KEY,
    webhook_event_id                    BIGINT REFERENCES webhook_events(id) ON DELETE CASCADE,
    webhook_event_trigger_evaluation_id BIGINT REFERENCES webhook_event_trigger_evaluations(id) ON DELETE CASCADE,
    repository_id                       BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    repository_trigger_id               BIGINT REFERENCES repository_triggers(id) ON DELETE CASCADE,
    pull_request_id                     BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    runtime_environment_id              BIGINT REFERENCES runtime_environments(id),
    target_runtime_environment_id       BIGINT REFERENCES runtime_environments(id),
    operation_type                      TEXT NOT NULL,
    source                              TEXT NOT NULL CHECK (source IN ('trigger', 'system')),
    status                              TEXT NOT NULL CHECK (status IN ('queued', 'handled', 'failed')),
    target_pr_head_commit_sha           TEXT NOT NULL,
    intent_snapshot_json                JSONB NOT NULL,
    outcome                             TEXT,
    last_error                          TEXT,
    handled_at                          TIMESTAMPTZ,
    created_at                          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX pull_requests_repository_id_idx
    ON pull_requests (repository_id, id ASC);

CREATE INDEX runtime_environments_pull_request_type_idx
    ON runtime_environments (pull_request_id, type, id DESC);

CREATE INDEX runtime_environments_target_idx
    ON runtime_environments (repository_id, pull_request_id, type, target_pr_head_commit_sha, id DESC);

CREATE INDEX operation_requests_webhook_event_id_idx
    ON operation_requests (webhook_event_id, id ASC);

CREATE INDEX operation_requests_status_idx
    ON operation_requests (status, id ASC);

CREATE INDEX operation_requests_pull_request_id_idx
    ON operation_requests (pull_request_id, id ASC);

CREATE INDEX operation_requests_runtime_environment_id_idx
    ON operation_requests (runtime_environment_id, id ASC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE operation_requests;
DROP TABLE runtime_environments;
DROP TABLE pull_requests;
-- +goose StatementEnd
