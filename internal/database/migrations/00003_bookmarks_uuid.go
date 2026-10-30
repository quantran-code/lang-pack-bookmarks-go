// Package migrations holds the database migrations: the goose SQL files in this directory, which
// internal/database embeds, and the Go migration that moves bookmarks from an auto-incrementing
// integer id to a UUID, because a fresh UUID per existing row can only be produced in Go.
package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

const (
	// createBookmarksUUIDTable builds the replacement bookmarks table. legacy_id remembers the
	// integer id a row had, so the down migration can restore it; no query selects it.
	createBookmarksUUIDTable = `
CREATE TABLE bookmarks_uuid (
    id         TEXT PRIMARY KEY NOT NULL,
    url        TEXT NOT NULL,
    title      TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    legacy_id  INTEGER UNIQUE
)`

	// createBookmarkTagsUUIDTable rebuilds the child table against the UUID id.
	createBookmarkTagsUUIDTable = `
CREATE TABLE bookmark_tags (
    bookmark_id TEXT    NOT NULL REFERENCES bookmarks (id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (bookmark_id, tag_id)
)`

	// createBookmarksIntegerTable is the pre-migration bookmarks table, rebuilt by the down
	// migration.
	createBookmarksIntegerTable = `
CREATE TABLE bookmarks_legacy (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    url        TEXT      NOT NULL,
    title      TEXT      NOT NULL,
    created_at TIMESTAMP NOT NULL
)`

	// createBookmarkTagsIntegerTable restores the child table against the integer id.
	createBookmarkTagsIntegerTable = `
CREATE TABLE bookmark_tags (
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks (id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (bookmark_id, tag_id)
)`
)

// integerBookmark is a bookmark as the up migration finds it. created_at is held as the driver
// returned it, so re-inserting it cannot reformat the timestamp.
type integerBookmark struct {
	id        int64
	url       string
	title     string
	createdAt any
}

// uuidBookmark is a bookmark as the down migration finds it. createdAt is held as the driver
// returned it, and legacyID is the integer id the bookmark had before the up migration, which is
// NULL for a bookmark created since.
type uuidBookmark struct {
	id        string
	url       string
	title     string
	createdAt any
	legacyID  sql.NullInt64
}

// integerTagLink is one bookmark_tags row while bookmark ids are still integers.
type integerTagLink struct {
	bookmarkID int64
	tagID      int64
}

// uuidTagLink is one bookmark_tags row after bookmark ids became UUIDs.
type uuidTagLink struct {
	bookmarkID string
	tagID      int64
}

// UpBookmarksUUID is goose migration 3 up: it rebuilds bookmarks and bookmark_tags with a UUID text
// id in place of the auto-incrementing integer. Every existing bookmark keeps its url, title and
// created_at, receives a fresh uuid.NewString(), and remembers the integer id it replaced in
// legacy_id, so DownBookmarksUUID can put that id back. The tags of a bookmark survive the rebuild,
// re-pointed through the legacy id.
//
// bookmark_tags is dropped before bookmarks, so no child row is ever left pointing at a table that
// no longer exists. That order is what lets the rebuild run with PRAGMA foreign_keys = ON: the
// pragma cannot be switched off inside a transaction, and the migration runs in goose's.
func UpBookmarksUUID(ctx context.Context, tx *sql.Tx) error {
	bookmarks, err := integerBookmarks(ctx, tx)
	if err != nil {
		return err
	}
	links, err := integerTagLinks(ctx, tx)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, createBookmarksUUIDTable); err != nil {
		return fmt.Errorf("creating bookmarks_uuid: %w", err)
	}

	uuids := make(map[int64]string, len(bookmarks))
	for _, bookmark := range bookmarks {
		id := uuid.NewString()
		uuids[bookmark.id] = id
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO bookmarks_uuid (id, url, title, created_at, legacy_id) VALUES (?, ?, ?, ?, ?)",
			id, bookmark.url, bookmark.title, bookmark.createdAt, bookmark.id,
		); err != nil {
			return fmt.Errorf("inserting bookmark %d as %s: %w", bookmark.id, id, err)
		}
	}

	if _, err := tx.ExecContext(ctx, "DROP TABLE bookmark_tags"); err != nil {
		return fmt.Errorf("dropping bookmark_tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE bookmarks"); err != nil {
		return fmt.Errorf("dropping bookmarks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE bookmarks_uuid RENAME TO bookmarks"); err != nil {
		return fmt.Errorf("renaming bookmarks_uuid to bookmarks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, createBookmarkTagsUUIDTable); err != nil {
		return fmt.Errorf("creating bookmark_tags: %w", err)
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)",
			uuids[link.bookmarkID], link.tagID,
		); err != nil {
			return fmt.Errorf("linking tag %d to bookmark %d: %w", link.tagID, link.bookmarkID, err)
		}
	}
	return nil
}

// DownBookmarksUUID is goose migration 3 down: it rebuilds bookmarks and bookmark_tags with the
// integer id and drops legacy_id. A bookmark that carries a legacy_id gets that id back, so the
// original ids and their order return exactly; a bookmark created after the up migration — so with
// no legacy_id — is given ids after every restored one, in created_at then id order. The tags of a
// bookmark survive the rebuild, re-pointed through the UUID.
func DownBookmarksUUID(ctx context.Context, tx *sql.Tx) error {
	var maxLegacyID sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT MAX(legacy_id) FROM bookmarks").Scan(&maxLegacyID); err != nil {
		return fmt.Errorf("reading the largest legacy id: %w", err)
	}
	nextID := maxLegacyID.Int64

	bookmarks, err := uuidBookmarks(ctx, tx)
	if err != nil {
		return err
	}
	links, err := uuidTagLinks(ctx, tx)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, createBookmarksIntegerTable); err != nil {
		return fmt.Errorf("creating bookmarks_legacy: %w", err)
	}

	ids := make(map[string]int64, len(bookmarks))
	for _, bookmark := range bookmarks {
		id := nextID + 1
		if bookmark.legacyID.Valid {
			id = bookmark.legacyID.Int64
		} else {
			nextID = id
		}
		ids[bookmark.id] = id
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO bookmarks_legacy (id, url, title, created_at) VALUES (?, ?, ?, ?)",
			id, bookmark.url, bookmark.title, bookmark.createdAt,
		); err != nil {
			return fmt.Errorf("restoring bookmark %s as %d: %w", bookmark.id, id, err)
		}
	}

	if _, err := tx.ExecContext(ctx, "DROP TABLE bookmark_tags"); err != nil {
		return fmt.Errorf("dropping bookmark_tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE bookmarks"); err != nil {
		return fmt.Errorf("dropping bookmarks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE bookmarks_legacy RENAME TO bookmarks"); err != nil {
		return fmt.Errorf("renaming bookmarks_legacy to bookmarks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, createBookmarkTagsIntegerTable); err != nil {
		return fmt.Errorf("creating bookmark_tags: %w", err)
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)",
			ids[link.bookmarkID], link.tagID,
		); err != nil {
			return fmt.Errorf("linking tag %d to bookmark %s: %w", link.tagID, link.bookmarkID, err)
		}
	}
	return nil
}

// integerBookmarks reads every bookmark in the integer-id table, in id order.
func integerBookmarks(ctx context.Context, tx *sql.Tx) ([]integerBookmark, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, url, title, created_at FROM bookmarks ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("listing bookmarks: %w", err)
	}
	defer rows.Close()

	var bookmarks []integerBookmark
	for rows.Next() {
		var bookmark integerBookmark
		if err := rows.Scan(&bookmark.id, &bookmark.url, &bookmark.title, &bookmark.createdAt); err != nil {
			return nil, fmt.Errorf("scanning bookmark: %w", err)
		}
		bookmarks = append(bookmarks, bookmark)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing bookmarks: %w", err)
	}
	return bookmarks, nil
}

// integerTagLinks reads every bookmark_tags row while bookmark ids are integers.
func integerTagLinks(ctx context.Context, tx *sql.Tx) ([]integerTagLink, error) {
	rows, err := tx.QueryContext(ctx, "SELECT bookmark_id, tag_id FROM bookmark_tags ORDER BY bookmark_id, tag_id")
	if err != nil {
		return nil, fmt.Errorf("listing bookmark tags: %w", err)
	}
	defer rows.Close()

	var links []integerTagLink
	for rows.Next() {
		var link integerTagLink
		if err := rows.Scan(&link.bookmarkID, &link.tagID); err != nil {
			return nil, fmt.Errorf("scanning bookmark tag: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing bookmark tags: %w", err)
	}
	return links, nil
}

// uuidBookmarks reads every bookmark in the UUID-id table in created_at then id order, which is the
// order the down migration hands out fresh integer ids in.
func uuidBookmarks(ctx context.Context, tx *sql.Tx) ([]uuidBookmark, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, url, title, created_at, legacy_id FROM bookmarks ORDER BY created_at, id")
	if err != nil {
		return nil, fmt.Errorf("listing bookmarks: %w", err)
	}
	defer rows.Close()

	var bookmarks []uuidBookmark
	for rows.Next() {
		var bookmark uuidBookmark
		if err := rows.Scan(&bookmark.id, &bookmark.url, &bookmark.title, &bookmark.createdAt, &bookmark.legacyID); err != nil {
			return nil, fmt.Errorf("scanning bookmark: %w", err)
		}
		bookmarks = append(bookmarks, bookmark)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing bookmarks: %w", err)
	}
	return bookmarks, nil
}

// uuidTagLinks reads every bookmark_tags row after bookmark ids became UUIDs.
func uuidTagLinks(ctx context.Context, tx *sql.Tx) ([]uuidTagLink, error) {
	rows, err := tx.QueryContext(ctx, "SELECT bookmark_id, tag_id FROM bookmark_tags ORDER BY bookmark_id, tag_id")
	if err != nil {
		return nil, fmt.Errorf("listing bookmark tags: %w", err)
	}
	defer rows.Close()

	var links []uuidTagLink
	for rows.Next() {
		var link uuidTagLink
		if err := rows.Scan(&link.bookmarkID, &link.tagID); err != nil {
			return nil, fmt.Errorf("scanning bookmark tag: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing bookmark tags: %w", err)
	}
	return links, nil
}
