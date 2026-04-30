-- name: GetPing :one
SELECT id, pinged_at FROM _ping ORDER BY id DESC LIMIT 1;

-- name: InsertPing :one
INSERT INTO _ping DEFAULT VALUES
RETURNING id, pinged_at;
