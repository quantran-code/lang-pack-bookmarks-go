package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// writableBody is a POST /bookmarks body that is always accepted, so a spec about the write
	// limit only has to care about the limit.
	writableBody = `{"url":"https://go.dev","title":"Go"}`
	// writeAddress is the connection address a spec that does not care which client is sending
	// uses.
	writeAddress = "10.0.0.1:4000"
)

// doFrom sends one request to the router from remoteAddr, carrying headers, and returns the
// recorded response. It exists because httptest.NewRequest gives every request the same
// 192.0.2.1:1234 address, so the per-client counters could not be exercised with do alone.
func doFrom(h http.Handler, method, target, body, remoteAddr string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// expectTooManyRequests asserts that rec is the answer to a request over the write limit: a 429,
// the project's error shape and a Retry-After header holding a positive number of seconds.
func expectTooManyRequests(rec *httptest.ResponseRecorder) {
	Expect(rec.Code).To(Equal(http.StatusTooManyRequests), rec.Body.String())
	Expect(rec.Body.String()).To(MatchJSON(`{"error":"too many requests"}`))

	raw := rec.Header().Get("Retry-After")
	retryAfter, err := strconv.Atoi(raw)
	Expect(err).NotTo(HaveOccurred(), "Retry-After is not an integer: %q", raw)
	Expect(retryAfter).To(BeNumerically(">", 0))
}

var writeRateLimitSpecs = Describe("POST /bookmarks rate limit", func() {
	Describe("over the limit", func() {
		var router http.Handler

		BeforeEach(func() {
			router = newTestRouterWithConfig(Config{WriteRateLimit: 2})
		})

		It("admits two requests and answers the third with 429", func() {
			Expect(doFrom(router, http.MethodPost, "/bookmarks", writableBody, writeAddress, nil).Code).To(Equal(http.StatusCreated))
			Expect(doFrom(router, http.MethodPost, "/bookmarks", writableBody, writeAddress, nil).Code).To(Equal(http.StatusCreated))

			expectTooManyRequests(doFrom(router, http.MethodPost, "/bookmarks", writableBody, writeAddress, nil))
		})

		It("counts a malformed or invalid request against the budget", func() {
			Expect(doFrom(router, http.MethodPost, "/bookmarks", `{"url":`, writeAddress, nil).Code).To(Equal(http.StatusBadRequest))
			Expect(doFrom(router, http.MethodPost, "/bookmarks", `{"title":"Go"}`, writeAddress, nil).Code).To(Equal(http.StatusBadRequest))

			expectTooManyRequests(doFrom(router, http.MethodPost, "/bookmarks", writableBody, writeAddress, nil))
		})
	})

	Describe("the routes that are never limited", func() {
		var router http.Handler

		BeforeEach(func() {
			router = newTestRouterWithConfig(Config{WriteRateLimit: 1})
			Expect(doFrom(router, http.MethodPost, "/bookmarks", writableBody, writeAddress, nil).Code).To(Equal(http.StatusCreated))
		})

		It("serves GET /bookmarks and GET /bookmarks/{id} with the budget spent", func() {
			for i := 0; i < 5; i++ {
				rec := doFrom(router, http.MethodGet, "/bookmarks", "", writeAddress, nil)
				Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			}

			rec := doFrom(router, http.MethodGet, "/bookmarks/1", "", writeAddress, nil)
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(rec.Body.String()).To(ContainSubstring(`"title":"Go"`))
		})

		It("serves GET /health with the budget spent", func() {
			for i := 0; i < 5; i++ {
				rec := doFrom(router, http.MethodGet, "/health", "", writeAddress, nil)
				Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"status":"ok"}`))
			}
		})
	})

	Describe("separate counters per client", func() {
		var router http.Handler

		BeforeEach(func() {
			router = newTestRouterWithConfig(Config{WriteRateLimit: 1})
		})

		It("keys the limit on the connection address, so two addresses each get a budget", func() {
			Expect(doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.1:4000", nil).Code).To(Equal(http.StatusCreated))
			Expect(doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.2:4000", nil).Code).To(Equal(http.StatusCreated))

			expectTooManyRequests(doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.1:4000", nil))
		})

		It("does not split the budget on the client's source port", func() {
			Expect(doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.1:4000", nil).Code).To(Equal(http.StatusCreated))

			expectTooManyRequests(doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.1:4001", nil))
		})

		It("ignores X-Forwarded-For and X-Real-IP", func() {
			first := doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.1:4000", map[string]string{"X-Forwarded-For": "1.1.1.1"})
			Expect(first.Code).To(Equal(http.StatusCreated), first.Body.String())

			expectTooManyRequests(doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.1:4000", map[string]string{"X-Forwarded-For": "2.2.2.2"}))
			expectTooManyRequests(doFrom(router, http.MethodPost, "/bookmarks", writableBody, "10.0.0.1:4000", map[string]string{"X-Real-IP": "3.3.3.3"}))
		})
	})

	DescribeTable("a limit that turns limiting off",
		func(limit int) {
			router := newTestRouterWithConfig(Config{WriteRateLimit: limit})

			for i := 0; i < 5; i++ {
				rec := doFrom(router, http.MethodPost, "/bookmarks", writableBody, writeAddress, nil)
				Expect(rec.Code).To(Equal(http.StatusCreated), rec.Body.String())
			}
		},
		Entry("zero", 0),
		Entry("negative", -1),
	)
})
