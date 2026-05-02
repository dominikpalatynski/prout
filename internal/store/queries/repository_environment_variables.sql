-- name: ListRepositoryEnvironmentVariablesByRepositoryID :many
SELECT
    id,
    repository_id,
    name,
    value,
    created_at,
    updated_at
FROM repository_environment_variables
WHERE repository_id = $1
ORDER BY name ASC, id ASC;

-- name: DeleteRepositoryEnvironmentVariablesByRepositoryID :exec
DELETE FROM repository_environment_variables
WHERE repository_id = $1;

-- name: InsertRepositoryEnvironmentVariable :one
INSERT INTO repository_environment_variables (
    repository_id,
    name,
    value
) VALUES (
    $1, $2, $3
)
RETURNING
    id,
    repository_id,
    name,
    value,
    created_at,
    updated_at;
