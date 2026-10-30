-- name: CreateBookmark :one
INSERT INTO bookmarks (id, url, title, created_at)
VALUES (?, ?, ?, ?)
RETURNING id, url, title, created_at;

-- name: GetBookmark :one
SELECT id, url, title, created_at FROM bookmarks WHERE id = ?;

-- name: ListBookmarksByID :many
SELECT id, url, title, created_at FROM bookmarks ORDER BY id ASC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByIDDesc :many
SELECT id, url, title, created_at FROM bookmarks ORDER BY id DESC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByTitle :many
SELECT id, url, title, created_at FROM bookmarks ORDER BY title ASC, id ASC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByTitleDesc :many
SELECT id, url, title, created_at FROM bookmarks ORDER BY title DESC, id DESC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByCreatedAt :many
SELECT id, url, title, created_at FROM bookmarks ORDER BY created_at ASC, id ASC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByCreatedAtDesc :many
SELECT id, url, title, created_at FROM bookmarks ORDER BY created_at DESC, id DESC LIMIT @limit OFFSET @offset;

-- name: CountBookmarks :one
SELECT COUNT(*) FROM bookmarks;

-- name: UpdateBookmark :one
UPDATE bookmarks SET url = ?, title = ? WHERE id = ? RETURNING id, url, title, created_at;

-- name: DeleteBookmark :execrows
DELETE FROM bookmarks WHERE id = ?;
