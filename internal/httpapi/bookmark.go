package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/quantran-code/lang-pack-bookmarks-go/internal/database/dbgen"
)

const (
	maxTitleLength = 200
	maxURLLength   = 2048
)

// bookmarkRequest is the body accepted when a bookmark is created.
type bookmarkRequest struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// validate applies the rules a bookmark has: an absolute http or https URL of at most maxURLLength
// characters, and a title that is not blank and at most maxTitleLength characters.
func (r bookmarkRequest) validate() error {
	rawURL := strings.TrimSpace(r.URL)
	if rawURL == "" {
		return errors.New("url is required")
	}
	if len(rawURL) > maxURLLength {
		return errors.New("url must be at most 2048 characters")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("url must be an absolute http or https URL")
	}

	title := strings.TrimSpace(r.Title)
	if title == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(title) > maxTitleLength {
		return errors.New("title must be at most 200 characters")
	}
	return nil
}

// bookmarkResponse is a bookmark as every endpoint answers with it: the stored columns plus the
// bookmark's tags, sorted by name and never null.
type bookmarkResponse struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags"`
}

// toBookmarkResponse builds the response for a bookmark row and its tags. A nil tags slice is
// replaced with an empty one so the field marshals as [] and never as null.
func toBookmarkResponse(b dbgen.Bookmark, tags []string) bookmarkResponse {
	if tags == nil {
		tags = make([]string, 0)
	}
	return bookmarkResponse{
		ID:        b.ID,
		URL:       b.Url,
		Title:     b.Title,
		CreatedAt: b.CreatedAt,
		Tags:      tags,
	}
}

// bookmarkTags loads the tags of a single bookmark, ascending by name. It is used only where the
// response covers one bookmark; listing loads every bookmark's tags in one query instead.
func bookmarkTags(ctx context.Context, q *dbgen.Queries, id int64) ([]string, error) {
	tags, err := q.ListTagsForBookmark(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("listing tags for bookmark %d: %w", id, err)
	}
	return tags, nil
}

// createBookmark handles POST /bookmarks.
func createBookmark(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req bookmarkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		bookmark, err := q.CreateBookmark(r.Context(), dbgen.CreateBookmarkParams{
			Url:       strings.TrimSpace(req.URL),
			Title:     strings.TrimSpace(req.Title),
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "creating bookmark")
			return
		}
		tags, err := bookmarkTags(r.Context(), q, bookmark.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading tags")
			return
		}
		writeJSON(w, http.StatusCreated, toBookmarkResponse(bookmark, tags))
	}
}

// listBookmarks handles GET /bookmarks: every bookmark, oldest first, or — when a valid ?tag= is
// given — only the bookmarks carrying that tag. The slice is made up front so an empty result
// marshals as [] and not null. A tag value that is not a valid tag cannot match anything, so it
// answers [] without running a query. Tags come from one query for the whole page, so listing
// bookmarks never runs a query per bookmark.
func listBookmarks(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		responses := make([]bookmarkResponse, 0)

		var bookmarks []dbgen.Bookmark
		if values, ok := r.URL.Query()["tag"]; ok {
			name := ""
			if len(values) > 0 {
				name = values[0]
			}
			if !validTag(name) {
				writeJSON(w, http.StatusOK, responses)
				return
			}
			filtered, err := q.ListBookmarksByTag(r.Context(), name)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "listing bookmarks")
				return
			}
			bookmarks = filtered
		} else {
			listed, err := q.ListBookmarks(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "listing bookmarks")
				return
			}
			bookmarks = listed
		}

		tagsByBookmark, err := loadAllBookmarkTags(r.Context(), q)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading tags")
			return
		}
		for _, bookmark := range bookmarks {
			responses = append(responses, toBookmarkResponse(bookmark, tagsByBookmark[bookmark.ID]))
		}
		writeJSON(w, http.StatusOK, responses)
	}
}

// loadAllBookmarkTags loads every bookmark's tags in one query, grouped by bookmark id. The names
// inside each group are already ascending because the query orders by bookmark id and tag name.
func loadAllBookmarkTags(ctx context.Context, q *dbgen.Queries) (map[int64][]string, error) {
	rows, err := q.ListAllBookmarkTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing bookmark tags: %w", err)
	}
	tags := make(map[int64][]string, len(rows))
	for _, row := range rows {
		tags[row.BookmarkID] = append(tags[row.BookmarkID], row.Name)
	}
	return tags, nil
}

// updateBookmark handles PUT /bookmarks/{id}: it replaces an existing bookmark's url and title and
// answers with the updated bookmark. The id is parsed and the bookmark loaded before the body is
// read, so a missing bookmark is a 404 even when the body is malformed or fails validation.
func updateBookmark(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseBookmarkID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookmark id")
			return
		}

		if _, err := q.GetBookmark(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "loading bookmark")
			return
		}

		var req bookmarkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		bookmark, err := q.UpdateBookmark(r.Context(), dbgen.UpdateBookmarkParams{
			Url:   strings.TrimSpace(req.URL),
			Title: strings.TrimSpace(req.Title),
			ID:    id,
		})
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "updating bookmark")
			return
		}
		tags, err := bookmarkTags(r.Context(), q, bookmark.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading tags")
			return
		}
		writeJSON(w, http.StatusOK, toBookmarkResponse(bookmark, tags))
	}
}

// deleteBookmark handles DELETE /bookmarks/{id}: it removes an existing bookmark and answers 204
// with no body at all. A returned row count of zero covers both "never existed" and "already
// deleted", and is answered like a missing bookmark.
func deleteBookmark(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseBookmarkID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookmark id")
			return
		}

		deleted, err := q.DeleteBookmark(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "deleting bookmark")
			return
		}
		if deleted == 0 {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// parseBookmarkID reads the {id} path parameter of a bookmark route. A non-numeric or non-positive
// id is an error.
func parseBookmarkID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid bookmark id")
	}
	return id, nil
}

// getBookmark handles GET /bookmarks/{id}. A non-numeric id is a 400, a missing bookmark a 404.
func getBookmark(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseBookmarkID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookmark id")
			return
		}

		bookmark, err := q.GetBookmark(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading bookmark")
			return
		}
		tags, err := bookmarkTags(r.Context(), q, bookmark.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading tags")
			return
		}
		writeJSON(w, http.StatusOK, toBookmarkResponse(bookmark, tags))
	}
}
