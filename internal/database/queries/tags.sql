-- name: EnsureTag :exec
INSERT INTO tags (name) VALUES (@name) ON CONFLICT (name) DO NOTHING;

-- name: AddBookmarkTag :exec
INSERT INTO bookmark_tags (bookmark_id, tag_id) SELECT @bookmark_id, id FROM tags WHERE name = @tag_name;

-- name: DeleteBookmarkTags :exec
DELETE FROM bookmark_tags WHERE bookmark_id = @bookmark_id;

-- name: ListTagsForBookmark :many
SELECT tags.name FROM bookmark_tags JOIN tags ON tags.id = bookmark_tags.tag_id WHERE bookmark_tags.bookmark_id = @bookmark_id ORDER BY tags.name;

-- name: ListAllBookmarkTags :many
SELECT bookmark_tags.bookmark_id, tags.name FROM bookmark_tags JOIN tags ON tags.id = bookmark_tags.tag_id ORDER BY bookmark_tags.bookmark_id, tags.name;

-- name: ListBookmarksByTagByID :many
SELECT bookmarks.id, bookmarks.url, bookmarks.title, bookmarks.created_at FROM bookmarks JOIN bookmark_tags ON bookmark_tags.bookmark_id = bookmarks.id JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = @name ORDER BY bookmarks.id ASC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByTagByIDDesc :many
SELECT bookmarks.id, bookmarks.url, bookmarks.title, bookmarks.created_at FROM bookmarks JOIN bookmark_tags ON bookmark_tags.bookmark_id = bookmarks.id JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = @name ORDER BY bookmarks.id DESC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByTagByTitle :many
SELECT bookmarks.id, bookmarks.url, bookmarks.title, bookmarks.created_at FROM bookmarks JOIN bookmark_tags ON bookmark_tags.bookmark_id = bookmarks.id JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = @name ORDER BY bookmarks.title ASC, bookmarks.id ASC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByTagByTitleDesc :many
SELECT bookmarks.id, bookmarks.url, bookmarks.title, bookmarks.created_at FROM bookmarks JOIN bookmark_tags ON bookmark_tags.bookmark_id = bookmarks.id JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = @name ORDER BY bookmarks.title DESC, bookmarks.id DESC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByTagByCreatedAt :many
SELECT bookmarks.id, bookmarks.url, bookmarks.title, bookmarks.created_at FROM bookmarks JOIN bookmark_tags ON bookmark_tags.bookmark_id = bookmarks.id JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = @name ORDER BY bookmarks.created_at ASC, bookmarks.id ASC LIMIT @limit OFFSET @offset;

-- name: ListBookmarksByTagByCreatedAtDesc :many
SELECT bookmarks.id, bookmarks.url, bookmarks.title, bookmarks.created_at FROM bookmarks JOIN bookmark_tags ON bookmark_tags.bookmark_id = bookmarks.id JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = @name ORDER BY bookmarks.created_at DESC, bookmarks.id DESC LIMIT @limit OFFSET @offset;

-- name: CountBookmarksByTag :one
SELECT COUNT(*) FROM bookmarks JOIN bookmark_tags ON bookmark_tags.bookmark_id = bookmarks.id JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = @name;
