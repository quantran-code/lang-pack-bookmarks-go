// Package httpapi holds the chi router and the handlers behind it.
package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/quantran-code/lang-pack-bookmarks-go/internal/database/dbgen"
)

// errorBody is the one error shape every endpoint answers with.
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON answers with status and v encoded as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError answers with {"error": "<message>"} and the given status.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorBody{Error: message})
}

// Config is what the router needs on top of the database.
type Config struct {
	// WriteRateLimit is the number of POST /bookmarks requests one client IP may make in any
	// rolling 60-second window. Zero or less turns limiting off.
	WriteRateLimit int
}

// DefaultWriteRateLimit is the write rate limit used when WRITE_RATE_LIMIT is unset.
const DefaultWriteRateLimit = 60

// ConfigFromEnv reads the configuration from the environment. WRITE_RATE_LIMIT holds the number of
// POST /bookmarks requests one client IP may make per minute: unset or empty means
// DefaultWriteRateLimit, and zero or a negative number is returned as given, which turns limiting
// off. A value that is not an integer is an error.
func ConfigFromEnv() (Config, error) {
	cfg := Config{WriteRateLimit: DefaultWriteRateLimit}

	raw := os.Getenv("WRITE_RATE_LIMIT")
	if raw == "" {
		return cfg, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return Config{}, fmt.Errorf("WRITE_RATE_LIMIT %q is not an integer: %w", raw, err)
	}
	cfg.WriteRateLimit = limit
	return cfg, nil
}

// NewRouter builds the chi router with every route registered. Paths chi does not match and
// methods it does not allow are answered in the same error shape.
func NewRouter(db *sql.DB, cfg Config) http.Handler {
	queries := dbgen.New(db)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/bookmarks", func(r chi.Router) {
		r.With(writeRateLimiter(cfg.WriteRateLimit)).Post("/", createBookmark(queries))
		r.Get("/", listBookmarks(queries))
		r.Get("/{id}", getBookmark(queries))
		r.Put("/{id}", updateBookmark(queries))
		r.Put("/{id}/tags", replaceBookmarkTags(queries, db))
		r.Delete("/{id}", deleteBookmark(queries))
	})
	return r
}
