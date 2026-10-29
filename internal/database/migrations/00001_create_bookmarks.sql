-- +goose Up
CREATE TABLE bookmarks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    url        TEXT      NOT NULL,
    title      TEXT      NOT NULL,
    created_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE bookmarks;
