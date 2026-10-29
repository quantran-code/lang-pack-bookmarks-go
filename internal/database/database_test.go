package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAppliesMigrations(t *testing.T) {
	db, err := Open(MemoryDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var version int
	require.NoError(t, db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version))
	assert.Equal(t, 1, version)

	_, err = db.Exec("INSERT INTO bookmarks (url, title, created_at) VALUES ('https://a', 'b', CURRENT_TIMESTAMP)")
	assert.NoError(t, err)
}

func TestOpenGivesEachCallItsOwnDatabase(t *testing.T) {
	first, err := Open(MemoryDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(MemoryDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	_, err = first.Exec("INSERT INTO bookmarks (url, title, created_at) VALUES ('https://a', 'b', CURRENT_TIMESTAMP)")
	require.NoError(t, err)

	var n int
	require.NoError(t, second.QueryRow("SELECT COUNT(*) FROM bookmarks").Scan(&n))
	assert.Equal(t, 0, n)
}
