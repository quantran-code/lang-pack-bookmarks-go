package database

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAppliesMigrations(t *testing.T) {
	db, err := Open(MemoryDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var version int
	require.NoError(t, db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version))
	assert.Equal(t, 3, version)

	bookmarkID := uuid.NewString()
	_, err = db.Exec("INSERT INTO bookmarks (id, url, title, created_at) VALUES (?, 'https://a', 'b', CURRENT_TIMESTAMP)", bookmarkID)
	require.NoError(t, err)

	result, err := db.Exec("INSERT INTO tags (name) VALUES ('go')")
	require.NoError(t, err)
	tagID, err := result.LastInsertId()
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)", bookmarkID, tagID)
	assert.NoError(t, err)
}

func TestDeleteBookmarkCascadesTagLinks(t *testing.T) {
	db, err := Open(MemoryDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	bookmarkID := uuid.NewString()
	_, err = db.Exec("INSERT INTO bookmarks (id, url, title, created_at) VALUES (?, 'https://a', 'b', CURRENT_TIMESTAMP)", bookmarkID)
	require.NoError(t, err)

	result, err := db.Exec("INSERT INTO tags (name) VALUES ('go')")
	require.NoError(t, err)
	tagID, err := result.LastInsertId()
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)", bookmarkID, tagID)
	require.NoError(t, err)

	_, err = db.Exec("DELETE FROM bookmarks WHERE id = ?", bookmarkID)
	require.NoError(t, err)

	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM bookmark_tags").Scan(&n))
	assert.Equal(t, 0, n)
}

func TestOpenGivesEachCallItsOwnDatabase(t *testing.T) {
	first, err := Open(MemoryDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(MemoryDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	_, err = first.Exec("INSERT INTO bookmarks (id, url, title, created_at) VALUES (?, 'https://a', 'b', CURRENT_TIMESTAMP)", uuid.NewString())
	require.NoError(t, err)

	var n int
	require.NoError(t, second.QueryRow("SELECT COUNT(*) FROM bookmarks").Scan(&n))
	assert.Equal(t, 0, n)
}
