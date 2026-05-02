-- name: GetRuntimeEnvironmentDeploymentByRuntimeEnvironmentID :one
SELECT
    runtime_environment_id,
    backend,
    frozen_runtime_settings_json,
    frozen_environment_variables_json,
    deployment_metadata_json,
    created_at,
    updated_at
FROM runtime_environment_deployments
WHERE runtime_environment_id = $1;

-- name: UpsertRuntimeEnvironmentDeployment :one
INSERT INTO runtime_environment_deployments (
    runtime_environment_id,
    backend,
    frozen_runtime_settings_json,
    frozen_environment_variables_json,
    deployment_metadata_json
) VALUES (
    $1, $2, $3, $4, $5
)
ON CONFLICT (runtime_environment_id) DO UPDATE
SET
    backend = EXCLUDED.backend,
    frozen_runtime_settings_json = EXCLUDED.frozen_runtime_settings_json,
    frozen_environment_variables_json = EXCLUDED.frozen_environment_variables_json,
    deployment_metadata_json = EXCLUDED.deployment_metadata_json,
    updated_at = NOW()
RETURNING
    runtime_environment_id,
    backend,
    frozen_runtime_settings_json,
    frozen_environment_variables_json,
    deployment_metadata_json,
    created_at,
    updated_at;
