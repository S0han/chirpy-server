-- name: GetAllChirps :many
SELECT * 
FROM chirps 
ORDER BY created_at ASC;

-- name: GetChirpsByUserID :many
SELECT id, created_at, updated_at, body, user_id
FROM chirps
WHERE user_id = $1
ORDER BY created_at ASC;