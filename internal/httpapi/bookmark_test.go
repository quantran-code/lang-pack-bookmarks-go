package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quantran-code/lang-pack-bookmarks-go/internal/database/dbgen"
)

var _ = Describe("bookmarks", func() {
	var router http.Handler

	BeforeEach(func() {
		router = newTestRouter()
	})

	Describe("GET /health", func() {
		It("answers ok", func() {
			rec := do(router, http.MethodGet, "/health", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("POST /bookmarks", func() {
		DescribeTable("creates or rejects",
			func(body string, status int, message string) {
				rec := do(router, http.MethodPost, "/bookmarks", body)
				Expect(rec.Code).To(Equal(status), rec.Body.String())
				if message != "" {
					Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
					return
				}
				var bookmark dbgen.Bookmark
				Expect(json.Unmarshal(rec.Body.Bytes(), &bookmark)).To(Succeed())
				Expect(bookmark.ID).To(BeEquivalentTo(1))
				Expect(bookmark.CreatedAt.IsZero()).To(BeFalse())
			},
			Entry("valid", `{"url":"https://go.dev","title":"Go"}`, http.StatusCreated, ""),
			Entry("http is allowed", `{"url":"http://example.com/a?b=c","title":"x"}`, http.StatusCreated, ""),
			Entry("title may be 200 characters", `{"url":"https://go.dev","title":"`+strings.Repeat("a", 200)+`"}`, http.StatusCreated, ""),
			Entry("missing url", `{"title":"Go"}`, http.StatusBadRequest, "url is required"),
			Entry("relative url", `{"url":"/docs","title":"Go"}`, http.StatusBadRequest, "url must be an absolute http or https URL"),
			Entry("other scheme", `{"url":"ftp://example.com","title":"Go"}`, http.StatusBadRequest, "url must be an absolute http or https URL"),
			Entry("missing title", `{"url":"https://go.dev"}`, http.StatusBadRequest, "title is required"),
			Entry("blank title", `{"url":"https://go.dev","title":"   "}`, http.StatusBadRequest, "title is required"),
			Entry("title too long", `{"url":"https://go.dev","title":"`+strings.Repeat("a", 201)+`"}`, http.StatusBadRequest, "title must be at most 200 characters"),
			Entry("malformed JSON", `{"url":`, http.StatusBadRequest, "invalid JSON body"),
		)
	})

	Describe("GET /bookmarks", func() {
		It("is [] when empty and lists oldest first", func() {
			rec := do(router, http.MethodGet, "/bookmarks", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(MatchJSON(`[]`))

			Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://a.example","title":"first"}`).Code).To(Equal(http.StatusCreated))
			Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://b.example","title":"second"}`).Code).To(Equal(http.StatusCreated))

			var bookmarks []dbgen.Bookmark
			rec = do(router, http.MethodGet, "/bookmarks", "")
			Expect(json.Unmarshal(rec.Body.Bytes(), &bookmarks)).To(Succeed())
			Expect(bookmarks).To(HaveLen(2))
			Expect(bookmarks[0].Title).To(Equal("first"))
			Expect(bookmarks[1].Title).To(Equal("second"))
		})
	})

	Describe("GET /bookmarks/{id}", func() {
		BeforeEach(func() {
			Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://go.dev","title":"Go"}`).Code).To(Equal(http.StatusCreated))
		})

		DescribeTable("answers",
			func(target string, status int, message string) {
				rec := do(router, http.MethodGet, target, "")
				Expect(rec.Code).To(Equal(status))
				if message != "" {
					Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
				}
			},
			Entry("found", "/bookmarks/1", http.StatusOK, ""),
			Entry("missing", "/bookmarks/99", http.StatusNotFound, "bookmark not found"),
			Entry("not a number", "/bookmarks/abc", http.StatusBadRequest, "invalid bookmark id"),
			Entry("zero", "/bookmarks/0", http.StatusBadRequest, "invalid bookmark id"),
			Entry("unknown path", "/nope", http.StatusNotFound, "not found"),
		)

		It("rejects a method the route does not have", func() {
			rec := do(router, http.MethodPatch, "/bookmarks/1", "")
			Expect(rec.Code).To(Equal(http.StatusMethodNotAllowed))
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"method not allowed"}`))
		})
	})
})
