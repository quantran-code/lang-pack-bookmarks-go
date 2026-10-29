-- name: CreateBookmark :one
INSERT INTO bookmarks (url, title, created_at)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetBookmark :one
SELECT * FROM bookmarks WHERE id = ?;

-- name: ListBookmarks :many
SELECT * FROM bookmarks ORDER BY id;
