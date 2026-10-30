package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"

	"github.com/quantran-code/lang-pack-bookmarks-go/internal/database/dbgen"
)

const (
	// maxTagsPerBookmark is how many distinct tags a bookmark may carry.
	maxTagsPerBookmark = 10
	// maxTagLength is the longest tag name allowed.
	maxTagLength = 30
)

// tagPattern is the one place the tag alphabet and length rule are expressed. Both the body of
// PUT /bookmarks/{id}/tags and the ?tag= filter are checked against it.
var tagPattern = regexp.MustCompile(`^[a-z0-9-]{1,30}$`)

// tagsRequest is the body accepted by PUT /bookmarks/{id}/tags. Tags is a pointer so a missing key
// and an explicit null (both nil) are told apart from an empty array, which clears every tag.
type tagsRequest struct {
	Tags *[]string `json:"tags"`
}

// validTag reports whether name is a tag: 1 to maxTagLength characters from a-z, 0-9 and -.
func validTag(name string) bool {
	if len(name) > maxTagLength {
		return false
	}
	return tagPattern.MatchString(name)
}

// normalizeTags applies the rules a tag set has: every name must be a valid tag, duplicates count
// once, and at most maxTagsPerBookmark distinct names are allowed. The format rule is checked over
// every name before the count is tested, so a request that breaks both is answered with the format
// message. The distinct names come back sorted ascending.
func normalizeTags(tags []string) ([]string, error) {
	for _, tag := range tags {
		if !validTag(tag) {
			return nil, errors.New("tag must be 1 to 30 characters from a-z, 0-9 and -")
		}
	}

	seen := make(map[string]struct{}, len(tags))
	distinct := make([]string, 0, len(tags))
	for _, tag := range tags {
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		distinct = append(distinct, tag)
	}
	if len(distinct) > maxTagsPerBookmark {
		return nil, errors.New("a bookmark has at most 10 tags")
	}

	sort.Strings(distinct)
	return distinct, nil
}

// replaceBookmarkTags handles PUT /bookmarks/{id}/tags: it replaces the bookmark's whole tag set
// with the one in the body and answers with the bookmark. The id is parsed and the bookmark loaded
// before the body is read, so a path value that is not a UUID is a 400 and a UUID that is not
// stored is a 404, both even when the body is malformed. The replacement runs in one transaction,
// so a failure leaves the previous tags in place.
func replaceBookmarkTags(q *dbgen.Queries, db *sql.DB) http.HandlerFunc {
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

		var req tagsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.Tags == nil {
			writeError(w, http.StatusBadRequest, "tags is required")
			return
		}
		tags, err := normalizeTags(*req.Tags)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// The whole set is replaced inside one transaction: a failure part way through rolls back
		// and leaves the bookmark's previous tags untouched.
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "updating tags")
			return
		}
		defer func() { _ = tx.Rollback() }()

		qtx := q.WithTx(tx)
		if err := qtx.DeleteBookmarkTags(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "updating tags")
			return
		}
		for _, tag := range tags {
			if err := qtx.EnsureTag(r.Context(), tag); err != nil {
				writeError(w, http.StatusInternalServerError, "updating tags")
				return
			}
			if err := qtx.AddBookmarkTag(r.Context(), dbgen.AddBookmarkTagParams{
				BookmarkID: id,
				TagName:    tag,
			}); err != nil {
				writeError(w, http.StatusInternalServerError, "updating tags")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "updating tags")
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
		storedTags, err := bookmarkTags(r.Context(), q, bookmark.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading tags")
			return
		}
		writeJSON(w, http.StatusOK, toBookmarkResponse(bookmarkModel(bookmark.ID, bookmark.Url, bookmark.Title, bookmark.CreatedAt), storedTags))
	}
}
