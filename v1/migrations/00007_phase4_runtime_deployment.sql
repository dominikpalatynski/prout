-- +goose Up
-- +goose StatementBegin
CREATE TABLE repository_runtime_settings (
    repository_id BIGINT PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    compose_file_path TEXT,
    exposed_service_name TEXT,
    exposed_service_port INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT repository_runtime_settings_exposed_service_port_check
        CHECK (exposed_service_port IS NULL OR exposed_service_port BETWEEN 1 AND 65535)
);

INSERT INTO repository_runtime_settings (repository_id)
SELECT repositories.id
FROM repositories
ON CONFLICT (repository_id) DO NOTHING;

CREATE TABLE repository_environment_variables (
    id BIGSERIAL PRIMARY KEY,
    repository_id BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT repository_environment_variables_name_not_blank_check
        CHECK (btrim(name) <> ''),
    CONSTRAINT repository_environment_variables_repository_id_name_unique
        UNIQUE (repository_id, name)
);

CREATE TABLE runtime_environment_deployments (
    runtime_environment_id BIGINT PRIMARY KEY REFERENCES runtime_environments(id) ON DELETE CASCADE,
    backend TEXT NOT NULL,
    frozen_runtime_settings_json JSONB NOT NULL,
    frozen_environment_variables_json JSONB NOT NULL,
    deployment_metadata_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE runtime_environment_deployments;
DROP TABLE repository_environment_variables;
DROP TABLE repository_runtime_settings;
-- +goose StatementEnd
