// Package httpapi holds the chi router and the handlers behind it.
package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"

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

// NewRouter builds the chi router with every route registered. Paths chi does not match and
// methods it does not allow are answered in the same error shape.
func NewRouter(db *sql.DB) http.Handler {
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
		r.Post("/", createBookmark(queries))
		r.Get("/", listBookmarks(queries))
		r.Get("/{id}", getBookmark(queries))
		r.Put("/{id}", updateBookmark(queries))
		r.Delete("/{id}", deleteBookmark(queries))
	})
	return r
}
