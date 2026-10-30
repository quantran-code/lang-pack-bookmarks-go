package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quantran-code/lang-pack-bookmarks-go/internal/database"
)

// newTestRouter returns a router backed by a fresh, migrated in-memory database and carrying the
// default write rate limit, so no spec depends on the developer's environment.
func newTestRouter() http.Handler {
	return newTestRouterWithConfig(Config{WriteRateLimit: DefaultWriteRateLimit})
}

// newTestRouterWithConfig returns a router configured with cfg, backed by a fresh, migrated
// in-memory database.
func newTestRouterWithConfig(cfg Config) http.Handler {
	router, _ := newTestRouterWithDBWithConfig(cfg)
	return router
}

// newTestRouterWithDB returns a router backed by a fresh, migrated in-memory database together
// with that database, so a spec can set up state the API cannot express — two rows sharing one
// created_at, for instance. It carries the default write rate limit.
func newTestRouterWithDB() (http.Handler, *sql.DB) {
	return newTestRouterWithDBWithConfig(Config{WriteRateLimit: DefaultWriteRateLimit})
}

// newTestRouterWithDBWithConfig is newTestRouterWithDB with an explicit configuration.
func newTestRouterWithDBWithConfig(cfg Config) (http.Handler, *sql.DB) {
	db, err := database.Open(database.MemoryDSN)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(db.Close)
	return NewRouter(db, cfg), db
}

// do sends one request to the router and returns the recorded response.
func do(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
