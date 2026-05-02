-- +goose Up
-- +goose StatementBegin
ALTER TABLE pull_requests
    ADD COLUMN current_source_github_repository_id BIGINT,
    ADD COLUMN current_source_owner TEXT,
    ADD COLUMN current_source_name TEXT,
    ADD COLUMN current_source_full_name TEXT;

UPDATE pull_requests
SET
    current_source_github_repository_id = repositories.github_repository_id,
    current_source_owner = repositories.owner,
    current_source_name = repositories.name,
    current_source_full_name = repositories.full_name
FROM repositories
WHERE repositories.id = pull_requests.repository_id;

ALTER TABLE pull_requests
    ALTER COLUMN current_source_github_repository_id SET NOT NULL,
    ALTER COLUMN current_source_owner SET NOT NULL,
    ALTER COLUMN current_source_name SET NOT NULL,
    ALTER COLUMN current_source_full_name SET NOT NULL;

ALTER TABLE runtime_environments
    ADD COLUMN source_github_repository_id BIGINT,
    ADD COLUMN source_owner TEXT,
    ADD COLUMN source_name TEXT,
    ADD COLUMN source_full_name TEXT;

UPDATE runtime_environments
SET
    source_github_repository_id = pull_requests.current_source_github_repository_id,
    source_owner = pull_requests.current_source_owner,
    source_name = pull_requests.current_source_name,
    source_full_name = pull_requests.current_source_full_name
FROM pull_requests
WHERE pull_requests.id = runtime_environments.pull_request_id;

ALTER TABLE runtime_environments
    ALTER COLUMN source_github_repository_id SET NOT NULL,
    ALTER COLUMN source_owner SET NOT NULL,
    ALTER COLUMN source_name SET NOT NULL,
    ALTER COLUMN source_full_name SET NOT NULL,
    ADD COLUMN workspace_locator TEXT GENERATED ALWAYS AS ('runtime-environments/' || id::text) STORED;

ALTER TABLE operation_requests
    ADD COLUMN current_step TEXT NOT NULL DEFAULT 'source_materialization',
    ADD COLUMN current_step_state TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN current_step_details_json JSONB;

UPDATE operation_requests
SET
    current_step = CASE
        WHEN operation_type = 'preview-cleanup-superseded' THEN 'workspace_cleanup'
        ELSE 'source_materialization'
    END,
    current_step_state = CASE
        WHEN status = 'handled' THEN 'completed'
        WHEN status = 'failed' THEN 'failed'
        ELSE 'pending'
    END;

ALTER TABLE operation_requests
    ADD CONSTRAINT operation_requests_current_step_state_check
        CHECK (current_step_state IN ('pending', 'in_progress', 'completed', 'failed'));

ALTER TABLE operation_requests
    ALTER COLUMN current_step DROP DEFAULT,
    ALTER COLUMN current_step_state DROP DEFAULT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE operation_requests
    DROP CONSTRAINT operation_requests_current_step_state_check,
    DROP COLUMN current_step_details_json,
    DROP COLUMN current_step_state,
    DROP COLUMN current_step;

ALTER TABLE runtime_environments
    DROP COLUMN workspace_locator,
    DROP COLUMN source_full_name,
    DROP COLUMN source_name,
    DROP COLUMN source_owner,
    DROP COLUMN source_github_repository_id;

ALTER TABLE pull_requests
    DROP COLUMN current_source_full_name,
    DROP COLUMN current_source_name,
    DROP COLUMN current_source_owner,
    DROP COLUMN current_source_github_repository_id;
-- +goose StatementEnd
