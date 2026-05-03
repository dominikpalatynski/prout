-- +goose Up
-- +goose StatementBegin
ALTER TABLE runtime_environments
    DROP CONSTRAINT runtime_environments_status_check,
    ADD CONSTRAINT runtime_environments_status_check
        CHECK (status IN ('preparing', 'prepared', 'failed', 'superseded', 'deleted'));

INSERT INTO repository_event_families (
    repository_id,
    event_family_key,
    enabled
)
SELECT
    repositories.id,
    'pull-request-unlabeled',
    TRUE
FROM repositories
ON CONFLICT (repository_id, event_family_key) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM repository_event_families
WHERE event_family_key = 'pull-request-unlabeled';

UPDATE runtime_environments
SET status = 'superseded'
WHERE status = 'deleted';

ALTER TABLE runtime_environments
    DROP CONSTRAINT runtime_environments_status_check,
    ADD CONSTRAINT runtime_environments_status_check
        CHECK (status IN ('preparing', 'prepared', 'failed', 'superseded'));
-- +goose StatementEnd
