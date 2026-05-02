-- +goose Up
-- +goose StatementBegin
CREATE TABLE repository_event_families (
    id                  BIGSERIAL PRIMARY KEY,
    repository_id       BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    event_family_key    TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT repository_event_families_repository_event_family_unique
        UNIQUE (repository_id, event_family_key)
);

INSERT INTO repository_event_families (
    repository_id,
    event_family_key,
    enabled
)
SELECT
    repositories.id,
    seeded.event_family_key,
    TRUE
FROM repositories
CROSS JOIN (
    VALUES
        ('pull-request-opened'),
        ('pull-request-labeled'),
        ('pull-request-comment-created')
) AS seeded(event_family_key)
ON CONFLICT (repository_id, event_family_key) DO NOTHING;

UPDATE repository_triggers
SET type = CASE type
    WHEN 'pull_request_opened' THEN 'preview_on_pull_request_opened'
    WHEN 'pull_request_label' THEN 'preview_on_label_preview'
    WHEN 'pull_request_comment_command' THEN 'preview_on_comment_preview'
    ELSE type
END;

ALTER TABLE repository_triggers
    DROP CONSTRAINT repository_triggers_repository_identity_key_unique;

ALTER TABLE repository_triggers
    DROP COLUMN event_family,
    DROP COLUMN identity_key,
    DROP COLUMN config_json;

ALTER TABLE repository_triggers
    ADD CONSTRAINT repository_triggers_repository_type_unique
        UNIQUE (repository_id, type);

CREATE INDEX repository_event_families_repository_id_idx
    ON repository_event_families (repository_id);

CREATE INDEX repository_event_families_repository_enabled_idx
    ON repository_event_families (repository_id, enabled);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX repository_event_families_repository_enabled_idx;
DROP INDEX repository_event_families_repository_id_idx;

ALTER TABLE repository_triggers
    DROP CONSTRAINT repository_triggers_repository_type_unique;

ALTER TABLE repository_triggers
    ADD COLUMN event_family TEXT NOT NULL DEFAULT '',
    ADD COLUMN identity_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN config_json JSONB NOT NULL DEFAULT '{}'::JSONB;

UPDATE repository_triggers
SET
    type = CASE type
        WHEN 'preview_on_pull_request_opened' THEN 'pull_request_opened'
        WHEN 'preview_on_label_preview' THEN 'pull_request_label'
        WHEN 'preview_on_comment_preview' THEN 'pull_request_comment_command'
        ELSE type
    END,
    event_family = CASE type
        WHEN 'preview_on_pull_request_opened' THEN 'pull_request.opened'
        WHEN 'preview_on_label_preview' THEN 'pull_request.labeled'
        WHEN 'preview_on_comment_preview' THEN 'issue_comment.created'
        ELSE ''
    END,
    identity_key = CASE type
        WHEN 'preview_on_pull_request_opened' THEN 'pull_request_opened'
        WHEN 'preview_on_label_preview' THEN 'pull_request_label:preview'
        WHEN 'preview_on_comment_preview' THEN 'pull_request_comment_command:exact_first_line:/preview'
        ELSE type
    END,
    config_json = CASE type
        WHEN 'preview_on_pull_request_opened' THEN '{}'::JSONB
        WHEN 'preview_on_label_preview' THEN '{"label":"preview"}'::JSONB
        WHEN 'preview_on_comment_preview' THEN '{"matcher":"exact_first_line","command":"/preview"}'::JSONB
        ELSE '{}'::JSONB
    END;

ALTER TABLE repository_triggers
    ADD CONSTRAINT repository_triggers_repository_identity_key_unique
        UNIQUE (repository_id, identity_key);

DROP TABLE repository_event_families;
-- +goose StatementEnd
