package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
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
		writeJSON(w, http.StatusCreated, bookmark)
	}
}

// listBookmarks handles GET /bookmarks: every bookmark, oldest first. The slice is made up front
// so an empty table marshals as [] and not null.
func listBookmarks(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookmarks, err := q.ListBookmarks(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "listing bookmarks")
			return
		}
		if bookmarks == nil {
			bookmarks = make([]dbgen.Bookmark, 0)
		}
		writeJSON(w, http.StatusOK, bookmarks)
	}
}

// getBookmark handles GET /bookmarks/{id}. A non-numeric id is a 400, a missing bookmark a 404.
func getBookmark(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil || id < 1 {
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
		writeJSON(w, http.StatusOK, bookmark)
	}
}
