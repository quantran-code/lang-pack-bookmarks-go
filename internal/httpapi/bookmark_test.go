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

	Describe("PUT /bookmarks/{id}", func() {
		var before dbgen.Bookmark

		BeforeEach(func() {
			Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://go.dev","title":"Go"}`).Code).To(Equal(http.StatusCreated))

			rec := do(router, http.MethodGet, "/bookmarks/1", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(json.Unmarshal(rec.Body.Bytes(), &before)).To(Succeed())
		})

		It("replaces url and title and keeps id and created_at", func() {
			rec := do(router, http.MethodPut, "/bookmarks/1", `{"url":"https://go.dev/blog","title":"The Go Blog"}`)
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())

			var updated dbgen.Bookmark
			Expect(json.Unmarshal(rec.Body.Bytes(), &updated)).To(Succeed())
			Expect(updated.ID).To(Equal(before.ID))
			Expect(updated.CreatedAt).To(BeTemporally("==", before.CreatedAt))
			Expect(updated.Url).To(Equal("https://go.dev/blog"))
			Expect(updated.Title).To(Equal("The Go Blog"))

			rec = do(router, http.MethodGet, "/bookmarks/1", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			var fetched dbgen.Bookmark
			Expect(json.Unmarshal(rec.Body.Bytes(), &fetched)).To(Succeed())
			Expect(fetched.ID).To(Equal(before.ID))
			Expect(fetched.CreatedAt).To(BeTemporally("==", before.CreatedAt))
			Expect(fetched.Url).To(Equal("https://go.dev/blog"))
			Expect(fetched.Title).To(Equal("The Go Blog"))
		})

		DescribeTable("rejects a body with the same messages as POST",
			func(body, message string) {
				rec := do(router, http.MethodPut, "/bookmarks/1", body)
				Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
			},
			Entry("missing url", `{"title":"Go"}`, "url is required"),
			Entry("url too long", `{"url":"https://example.com/`+strings.Repeat("a", 2048)+`","title":"Go"}`, "url must be at most 2048 characters"),
			Entry("relative url", `{"url":"/docs","title":"Go"}`, "url must be an absolute http or https URL"),
			Entry("other scheme", `{"url":"ftp://example.com","title":"Go"}`, "url must be an absolute http or https URL"),
			Entry("missing title", `{"url":"https://go.dev"}`, "title is required"),
			Entry("blank title", `{"url":"https://go.dev","title":"   "}`, "title is required"),
			Entry("title too long", `{"url":"https://go.dev","title":"`+strings.Repeat("a", 201)+`"}`, "title must be at most 200 characters"),
			Entry("malformed JSON", `{"url":`, "invalid JSON body"),
		)

		It("leaves the bookmark alone when the id is invalid", func() {
			Expect(do(router, http.MethodPut, "/bookmarks/abc", `{"url":"https://x.example","title":"x"}`).Code).To(Equal(http.StatusBadRequest))
			Expect(do(router, http.MethodPut, "/bookmarks/0", `{"url":"https://x.example","title":"x"}`).Code).To(Equal(http.StatusBadRequest))

			rec := do(router, http.MethodGet, "/bookmarks/1", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			var after dbgen.Bookmark
			Expect(json.Unmarshal(rec.Body.Bytes(), &after)).To(Succeed())
			Expect(after.Url).To(Equal(before.Url))
			Expect(after.Title).To(Equal(before.Title))
		})

		DescribeTable("answers for a bad id or a missing bookmark",
			func(target, body string, status int, message string) {
				rec := do(router, http.MethodPut, target, body)
				Expect(rec.Code).To(Equal(status), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
			},
			Entry("not a number", "/bookmarks/abc", `{"url":"https://go.dev","title":"Go"}`, http.StatusBadRequest, "invalid bookmark id"),
			Entry("zero", "/bookmarks/0", `{"url":"https://go.dev","title":"Go"}`, http.StatusBadRequest, "invalid bookmark id"),
			Entry("missing", "/bookmarks/99", `{"url":"https://go.dev","title":"Go"}`, http.StatusNotFound, "bookmark not found"),
			Entry("missing, invalid body", "/bookmarks/99", `{"title":"Go"}`, http.StatusNotFound, "bookmark not found"),
			Entry("missing, malformed JSON", "/bookmarks/99", `{"url":`, http.StatusNotFound, "bookmark not found"),
		)
	})

	Describe("DELETE /bookmarks/{id}", func() {
		BeforeEach(func() {
			Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://a.example","title":"first"}`).Code).To(Equal(http.StatusCreated))
			Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://b.example","title":"second"}`).Code).To(Equal(http.StatusCreated))
		})

		It("answers 204 with no body and is gone afterwards", func() {
			rec := do(router, http.MethodDelete, "/bookmarks/1", "")
			Expect(rec.Code).To(Equal(http.StatusNoContent))
			Expect(rec.Body.String()).To(BeEmpty())

			rec = do(router, http.MethodGet, "/bookmarks/1", "")
			Expect(rec.Code).To(Equal(http.StatusNotFound))
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"bookmark not found"}`))
		})

		It("is a 404 the second time", func() {
			Expect(do(router, http.MethodDelete, "/bookmarks/1", "").Code).To(Equal(http.StatusNoContent))

			rec := do(router, http.MethodDelete, "/bookmarks/1", "")
			Expect(rec.Code).To(Equal(http.StatusNotFound))
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"bookmark not found"}`))
		})

		It("leaves the other bookmarks listed", func() {
			Expect(do(router, http.MethodDelete, "/bookmarks/1", "").Code).To(Equal(http.StatusNoContent))

			rec := do(router, http.MethodGet, "/bookmarks", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			var bookmarks []dbgen.Bookmark
			Expect(json.Unmarshal(rec.Body.Bytes(), &bookmarks)).To(Succeed())
			Expect(bookmarks).To(HaveLen(1))
			Expect(bookmarks[0].ID).To(BeEquivalentTo(2))
			Expect(bookmarks[0].Title).To(Equal("second"))
		})

		DescribeTable("answers for a bad id or a missing bookmark",
			func(target string, status int, message string) {
				rec := do(router, http.MethodDelete, target, "")
				Expect(rec.Code).To(Equal(status), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
			},
			Entry("not a number", "/bookmarks/abc", http.StatusBadRequest, "invalid bookmark id"),
			Entry("zero", "/bookmarks/0", http.StatusBadRequest, "invalid bookmark id"),
			Entry("missing", "/bookmarks/99", http.StatusNotFound, "bookmark not found"),
		)
	})
})
