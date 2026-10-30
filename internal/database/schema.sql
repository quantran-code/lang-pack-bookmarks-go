-- The schema sqlc reads to generate internal/database/dbgen. It mirrors what the migrations in
-- internal/database/migrations leave behind: the bookmarks table rebuilt on TEXT UUID ids by the
-- goose migration 3. The hidden legacy_id column the down migration uses is declared here too,
-- so this file is a faithful copy of the table the goose migration creates.
--
-- sqlc cannot read a goose Go migration, so this file is the second source of truth: a migration
-- that changes a table has to change this file too. Runtime DDL comes from the migrations, never
-- from here.

CREATE TABLE bookmarks (
    id         TEXT PRIMARY KEY NOT NULL,
    url        TEXT      NOT NULL,
    title      TEXT      NOT NULL,
    created_at TIMESTAMP NOT NULL,
    legacy_id  INTEGER UNIQUE
);

CREATE TABLE tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT      NOT NULL UNIQUE
);

CREATE TABLE bookmark_tags (
    bookmark_id TEXT    NOT NULL REFERENCES bookmarks (id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (bookmark_id, tag_id)
);
