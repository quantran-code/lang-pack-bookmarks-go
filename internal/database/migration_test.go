package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seededBookmark is a bookmark as a migration test inserts it and expects to find it again.
type seededBookmark struct {
	url       string
	title     string
	createdAt time.Time
}

// migratedBookmark is a bookmark read back after the UUID migration: same url, title and
// created_at as the seed, with a UUID id.
type migratedBookmark struct {
	id        string
	url       string
	title     string
	createdAt time.Time
}

// openMigrationTestDB opens an empty in-memory database without applying any migration, so a test
// can take it to one version and back. Foreign keys are on, as database.Open turns them on, so the
// migrations are exercised under the same constraints as in production.
func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", MemoryDSN)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	return db
}

// seedVersion2Bookmarks inserts two bookmarks with integer ids 1 and 2, one tag, and one tag link
// from the first bookmark to it.
func seedVersion2Bookmarks(t *testing.T, db *sql.DB, first, second time.Time) {
	t.Helper()

	ctx := context.Background()
	_, err := db.ExecContext(ctx, "INSERT INTO bookmarks (url, title, created_at) VALUES ('https://a.example', 'first', ?)", first)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO bookmarks (url, title, created_at) VALUES ('https://b.example', 'second', ?)", second)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO tags (name) VALUES ('go')")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO bookmark_tags (bookmark_id, tag_id) VALUES (1, 1)")
	require.NoError(t, err)
}

func TestUUIDMigrationKeepsExistingBookmarks(t *testing.T) {
	ctx := context.Background()
	db := openMigrationTestDB(t)
	provider, err := newMigrationProvider(db)
	require.NoError(t, err)
	_, err = provider.UpTo(ctx, 2)
	require.NoError(t, err)

	seeds := []seededBookmark{
		{url: "https://a.example", title: "first", createdAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		{url: "https://b.example", title: "second", createdAt: time.Date(2024, 1, 3, 3, 4, 5, 0, time.UTC)},
	}
	seedVersion2Bookmarks(t, db, seeds[0].createdAt, seeds[1].createdAt)

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	rows, err := db.QueryContext(ctx, "SELECT id, url, title, created_at FROM bookmarks ORDER BY created_at")
	require.NoError(t, err)
	var got []migratedBookmark
	for rows.Next() {
		var bookmark migratedBookmark
		require.NoError(t, rows.Scan(&bookmark.id, &bookmark.url, &bookmark.title, &bookmark.createdAt))
		got = append(got, bookmark)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	require.Len(t, got, len(seeds))
	for i, bookmark := range got {
		_, err := uuid.Parse(bookmark.id)
		require.NoError(t, err, "id %q is not a UUID", bookmark.id)
		assert.Equal(t, seeds[i].url, bookmark.url)
		assert.Equal(t, seeds[i].title, bookmark.title)
		assert.True(t, seeds[i].createdAt.Equal(bookmark.createdAt), "created_at moved from %s to %s", seeds[i].createdAt, bookmark.createdAt)
	}
	assert.NotEqual(t, got[0].id, got[1].id)

	var linkedTo string
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT bookmark_tags.bookmark_id FROM bookmark_tags JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = 'go'",
	).Scan(&linkedTo))
	assert.Equal(t, got[0].id, linkedTo)
}

func TestUUIDMigrationDownRestoresIntegerIDs(t *testing.T) {
	ctx := context.Background()
	db := openMigrationTestDB(t)
	provider, err := newMigrationProvider(db)
	require.NoError(t, err)
	_, err = provider.UpTo(ctx, 2)
	require.NoError(t, err)

	seeds := []seededBookmark{
		{url: "https://a.example", title: "first", createdAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		{url: "https://b.example", title: "second", createdAt: time.Date(2024, 1, 3, 3, 4, 5, 0, time.UTC)},
	}
	seedVersion2Bookmarks(t, db, seeds[0].createdAt, seeds[1].createdAt)

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	// A bookmark created after the up migration has no legacy id, so the down migration has to
	// invent one for it.
	_, err = db.ExecContext(ctx,
		"INSERT INTO bookmarks (id, url, title, created_at) VALUES (?, 'https://c.example', 'third', ?)",
		uuid.NewString(), seeds[1].createdAt.Add(24*time.Hour),
	)
	require.NoError(t, err)

	_, err = provider.DownTo(ctx, 2)
	require.NoError(t, err)

	var firstID, secondID, extraID int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT id FROM bookmarks WHERE url = 'https://a.example'").Scan(&firstID))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT id FROM bookmarks WHERE url = 'https://b.example'").Scan(&secondID))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT id FROM bookmarks WHERE url = 'https://c.example'").Scan(&extraID))
	assert.Equal(t, int64(1), firstID)
	assert.Equal(t, int64(2), secondID)
	assert.Greater(t, extraID, firstID)
	assert.Greater(t, extraID, secondID)

	var linkedTo int64
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT bookmark_tags.bookmark_id FROM bookmark_tags JOIN tags ON tags.id = bookmark_tags.tag_id WHERE tags.name = 'go'",
	).Scan(&linkedTo))
	assert.Equal(t, int64(1), linkedTo)

	var legacyID any
	require.Error(t, db.QueryRowContext(ctx, "SELECT legacy_id FROM bookmarks").Scan(&legacyID))
}
