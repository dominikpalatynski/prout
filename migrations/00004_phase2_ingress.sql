-- +goose Up
-- +goose StatementBegin
CREATE TABLE repositories (
    id                      BIGSERIAL PRIMARY KEY,
    github_repository_id    BIGINT NOT NULL UNIQUE,
    github_installation_id  BIGINT NOT NULL,
    owner                   TEXT NOT NULL,
    name                    TEXT NOT NULL,
    full_name               TEXT NOT NULL,
    html_url                TEXT NOT NULL,
    is_private              BOOLEAN NOT NULL DEFAULT FALSE,
    enabled                 BOOLEAN NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE repository_triggers (
    id              BIGSERIAL PRIMARY KEY,
    repository_id   BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    type            TEXT NOT NULL,
    event_family    TEXT NOT NULL,
    identity_key    TEXT NOT NULL,
    config_json     JSONB NOT NULL DEFAULT '{}'::JSONB,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT repository_triggers_repository_identity_key_unique
        UNIQUE (repository_id, identity_key)
);

CREATE TABLE webhook_events (
    id                      BIGSERIAL PRIMARY KEY,
    delivery_id             TEXT NOT NULL UNIQUE,
    github_event            TEXT NOT NULL,
    event_type              TEXT NOT NULL,
    repository_id           BIGINT REFERENCES repositories(id),
    github_repository_id    BIGINT,
    status                  TEXT NOT NULL CHECK (status IN ('ignored', 'processed', 'failed')),
    ignored_reason          TEXT,
    failure_message         TEXT,
    payload_json            JSONB NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE webhook_event_trigger_evaluations (
    id                      BIGSERIAL PRIMARY KEY,
    webhook_event_id        BIGINT NOT NULL REFERENCES webhook_events(id) ON DELETE CASCADE,
    repository_trigger_id   BIGINT NOT NULL REFERENCES repository_triggers(id) ON DELETE CASCADE,
    matched                 BOOLEAN NOT NULL,
    reason                  TEXT NOT NULL,
    trigger_snapshot_json   JSONB NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE trigger_dispatches (
    id                                  BIGSERIAL PRIMARY KEY,
    webhook_event_id                    BIGINT NOT NULL REFERENCES webhook_events(id) ON DELETE CASCADE,
    webhook_event_trigger_evaluation_id BIGINT NOT NULL REFERENCES webhook_event_trigger_evaluations(id) ON DELETE CASCADE,
    repository_id                       BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    repository_trigger_id               BIGINT NOT NULL REFERENCES repository_triggers(id) ON DELETE CASCADE,
    dispatch_type                       TEXT NOT NULL,
    status                              TEXT NOT NULL CHECK (status IN ('queued', 'processed', 'failed')),
    dispatch_payload_json               JSONB NOT NULL,
    last_error                          TEXT,
    processed_at                        TIMESTAMPTZ,
    created_at                          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX repository_triggers_repository_id_idx
    ON repository_triggers (repository_id);

CREATE INDEX repository_triggers_repository_enabled_idx
    ON repository_triggers (repository_id, enabled);

CREATE INDEX webhook_events_repository_id_idx
    ON webhook_events (repository_id, id DESC);

CREATE INDEX webhook_events_status_idx
    ON webhook_events (status, id DESC);

CREATE INDEX webhook_events_event_type_idx
    ON webhook_events (event_type, id DESC);

CREATE INDEX webhook_events_github_repository_id_idx
    ON webhook_events (github_repository_id, id DESC);

CREATE INDEX webhook_event_trigger_evaluations_webhook_event_id_idx
    ON webhook_event_trigger_evaluations (webhook_event_id, id ASC);

CREATE INDEX trigger_dispatches_webhook_event_id_idx
    ON trigger_dispatches (webhook_event_id, id ASC);

CREATE INDEX trigger_dispatches_status_idx
    ON trigger_dispatches (status, id ASC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE trigger_dispatches;
DROP TABLE webhook_event_trigger_evaluations;
DROP TABLE webhook_events;
DROP TABLE repository_triggers;
DROP TABLE repositories;
-- +goose StatementEnd
