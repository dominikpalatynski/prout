-- name: GetRepositoryRuntimeSettingsByRepositoryID :one
SELECT
    repository_id,
    compose_file_path,
    exposed_service_name,
    exposed_service_port,
    created_at,
    updated_at
FROM repository_runtime_settings
WHERE repository_id = $1;

-- name: UpsertRepositoryRuntimeSettings :one
INSERT INTO repository_runtime_settings (
    repository_id,
    compose_file_path,
    exposed_service_name,
    exposed_service_port
) VALUES (
    $1, $2, $3, $4
)
ON CONFLICT (repository_id) DO UPDATE
SET
    compose_file_path = EXCLUDED.compose_file_path,
    exposed_service_name = EXCLUDED.exposed_service_name,
    exposed_service_port = EXCLUDED.exposed_service_port,
    updated_at = NOW()
RETURNING
    repository_id,
    compose_file_path,
    exposed_service_name,
    exposed_service_port,
    created_at,
    updated_at;
