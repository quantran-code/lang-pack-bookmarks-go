package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// createdBookmarkID returns the id in the body of a POST /bookmarks response that answered 201,
// failing the spec when the id is not a UUID.
func createdBookmarkID(rec *httptest.ResponseRecorder) string {
	var created bookmarkResponse
	Expect(json.Unmarshal(rec.Body.Bytes(), &created)).To(Succeed())
	_, err := uuid.Parse(created.ID)
	Expect(err).NotTo(HaveOccurred(), "id %q is not a UUID", created.ID)
	return created.ID
}

// postBookmark creates one bookmark through the API and returns its id, so a spec can address it
// under /bookmarks/{id} instead of guessing a value.
func postBookmark(router http.Handler, bookmarkURL, title string) string {
	body, err := json.Marshal(map[string]string{"url": bookmarkURL, "title": title})
	Expect(err).NotTo(HaveOccurred())
	rec := do(router, http.MethodPost, "/bookmarks", string(body))
	Expect(rec.Code).To(Equal(http.StatusCreated), rec.Body.String())
	return createdBookmarkID(rec)
}

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
				var bookmark bookmarkResponse
				Expect(json.Unmarshal(rec.Body.Bytes(), &bookmark)).To(Succeed())
				_, err := uuid.Parse(bookmark.ID)
				Expect(err).NotTo(HaveOccurred(), "id %q is not a UUID", bookmark.ID)
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

		It("answers with the id as a JSON string", func() {
			rec := do(router, http.MethodPost, "/bookmarks", `{"url":"https://go.dev","title":"Go"}`)
			Expect(rec.Code).To(Equal(http.StatusCreated), rec.Body.String())

			var fields map[string]json.RawMessage
			Expect(json.Unmarshal(rec.Body.Bytes(), &fields)).To(Succeed())
			var id string
			Expect(json.Unmarshal(fields["id"], &id)).To(Succeed(), "id is not a JSON string: %s", string(fields["id"]))
			_, err := uuid.Parse(id)
			Expect(err).NotTo(HaveOccurred(), "id %q is not a UUID", id)
		})

		It("gives every new bookmark its own id, which reads back", func() {
			first := postBookmark(router, "https://a.example", "first")
			second := postBookmark(router, "https://b.example", "second")
			Expect(first).NotTo(Equal(second))

			rec := do(router, http.MethodGet, "/bookmarks/"+first, "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			var fetched bookmarkResponse
			Expect(json.Unmarshal(rec.Body.Bytes(), &fetched)).To(Succeed())
			Expect(fetched.ID).To(Equal(first))
			Expect(fetched.URL).To(Equal("https://a.example"))
			Expect(fetched.Title).To(Equal("first"))
		})
	})

	Describe("GET /bookmarks", func() {
		It("is [] when empty and lists oldest first", func() {
			rec := do(router, http.MethodGet, "/bookmarks", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(MatchJSON(`[]`))

			first := postBookmark(router, "https://a.example", "first")
			postBookmark(router, "https://b.example", "second")

			var bookmarks []bookmarkResponse
			rec = do(router, http.MethodGet, "/bookmarks", "")
			Expect(json.Unmarshal(rec.Body.Bytes(), &bookmarks)).To(Succeed())
			Expect(bookmarks).To(HaveLen(2))
			Expect(bookmarks[0].ID).To(Equal(first))
			Expect(bookmarks[0].Title).To(Equal("first"))
			Expect(bookmarks[1].Title).To(Equal("second"))
		})
	})

	Describe("GET /bookmarks/{id}", func() {
		var id string

		BeforeEach(func() {
			id = postBookmark(router, "https://go.dev", "Go")
		})

		It("answers the bookmark it created", func() {
			rec := do(router, http.MethodGet, "/bookmarks/"+id, "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())

			var found bookmarkResponse
			Expect(json.Unmarshal(rec.Body.Bytes(), &found)).To(Succeed())
			Expect(found.ID).To(Equal(id))
			Expect(found.URL).To(Equal("https://go.dev"))
			Expect(found.Title).To(Equal("Go"))
			Expect(found.CreatedAt.IsZero()).To(BeFalse())
			Expect(found.Tags).To(BeEmpty())
		})

		DescribeTable("answers",
			func(target string, status int, message string) {
				rec := do(router, http.MethodGet, target, "")
				Expect(rec.Code).To(Equal(status), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
			},
			Entry("missing", "/bookmarks/"+uuid.NewString(), http.StatusNotFound, "bookmark not found"),
			Entry("not a UUID", "/bookmarks/abc", http.StatusBadRequest, "invalid bookmark id"),
			Entry("zero", "/bookmarks/0", http.StatusBadRequest, "invalid bookmark id"),
			Entry("unknown path", "/nope", http.StatusNotFound, "not found"),
		)

		It("rejects a method the route does not have", func() {
			rec := do(router, http.MethodPatch, "/bookmarks/"+id, "")
			Expect(rec.Code).To(Equal(http.StatusMethodNotAllowed))
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"method not allowed"}`))
		})
	})

	Describe("PUT /bookmarks/{id}", func() {
		var id string
		var before bookmarkResponse

		BeforeEach(func() {
			id = postBookmark(router, "https://go.dev", "Go")

			rec := do(router, http.MethodGet, "/bookmarks/"+id, "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(json.Unmarshal(rec.Body.Bytes(), &before)).To(Succeed())
		})

		It("replaces url and title and keeps id and created_at", func() {
			rec := do(router, http.MethodPut, "/bookmarks/"+id, `{"url":"https://go.dev/blog","title":"The Go Blog"}`)
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())

			var updated bookmarkResponse
			Expect(json.Unmarshal(rec.Body.Bytes(), &updated)).To(Succeed())
			Expect(updated.ID).To(Equal(before.ID))
			Expect(updated.CreatedAt).To(BeTemporally("==", before.CreatedAt))
			Expect(updated.URL).To(Equal("https://go.dev/blog"))
			Expect(updated.Title).To(Equal("The Go Blog"))

			rec = do(router, http.MethodGet, "/bookmarks/"+id, "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			var fetched bookmarkResponse
			Expect(json.Unmarshal(rec.Body.Bytes(), &fetched)).To(Succeed())
			Expect(fetched.ID).To(Equal(before.ID))
			Expect(fetched.CreatedAt).To(BeTemporally("==", before.CreatedAt))
			Expect(fetched.URL).To(Equal("https://go.dev/blog"))
			Expect(fetched.Title).To(Equal("The Go Blog"))
		})

		DescribeTable("rejects a body with the same messages as POST",
			func(body, message string) {
				rec := do(router, http.MethodPut, "/bookmarks/"+id, body)
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

			rec := do(router, http.MethodGet, "/bookmarks/"+id, "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			var after bookmarkResponse
			Expect(json.Unmarshal(rec.Body.Bytes(), &after)).To(Succeed())
			Expect(after.URL).To(Equal(before.URL))
			Expect(after.Title).To(Equal(before.Title))
		})

		DescribeTable("answers for a bad id or a missing bookmark",
			func(target, body string, status int, message string) {
				rec := do(router, http.MethodPut, target, body)
				Expect(rec.Code).To(Equal(status), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
			},
			Entry("not a UUID", "/bookmarks/abc", `{"url":"https://go.dev","title":"Go"}`, http.StatusBadRequest, "invalid bookmark id"),
			Entry("zero", "/bookmarks/0", `{"url":"https://go.dev","title":"Go"}`, http.StatusBadRequest, "invalid bookmark id"),
			Entry("missing", "/bookmarks/"+uuid.NewString(), `{"url":"https://go.dev","title":"Go"}`, http.StatusNotFound, "bookmark not found"),
			Entry("missing, invalid body", "/bookmarks/"+uuid.NewString(), `{"title":"Go"}`, http.StatusNotFound, "bookmark not found"),
			Entry("missing, malformed JSON", "/bookmarks/"+uuid.NewString(), `{"url":`, http.StatusNotFound, "bookmark not found"),
		)
	})

	Describe("DELETE /bookmarks/{id}", func() {
		var first, second string

		BeforeEach(func() {
			first = postBookmark(router, "https://a.example", "first")
			second = postBookmark(router, "https://b.example", "second")
		})

		It("answers 204 with no body and is gone afterwards", func() {
			rec := do(router, http.MethodDelete, "/bookmarks/"+first, "")
			Expect(rec.Code).To(Equal(http.StatusNoContent))
			Expect(rec.Body.String()).To(BeEmpty())

			rec = do(router, http.MethodGet, "/bookmarks/"+first, "")
			Expect(rec.Code).To(Equal(http.StatusNotFound))
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"bookmark not found"}`))
		})

		It("is a 404 the second time", func() {
			Expect(do(router, http.MethodDelete, "/bookmarks/"+first, "").Code).To(Equal(http.StatusNoContent))

			rec := do(router, http.MethodDelete, "/bookmarks/"+first, "")
			Expect(rec.Code).To(Equal(http.StatusNotFound))
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"bookmark not found"}`))
		})

		It("leaves the other bookmarks listed", func() {
			Expect(do(router, http.MethodDelete, "/bookmarks/"+first, "").Code).To(Equal(http.StatusNoContent))

			rec := do(router, http.MethodGet, "/bookmarks", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			var bookmarks []bookmarkResponse
			Expect(json.Unmarshal(rec.Body.Bytes(), &bookmarks)).To(Succeed())
			Expect(bookmarks).To(HaveLen(1))
			Expect(bookmarks[0].ID).To(Equal(second))
			Expect(bookmarks[0].Title).To(Equal("second"))
		})

		DescribeTable("answers for a bad id or a missing bookmark",
			func(target string, status int, message string) {
				rec := do(router, http.MethodDelete, target, "")
				Expect(rec.Code).To(Equal(status), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
			},
			Entry("not a UUID", "/bookmarks/abc", http.StatusBadRequest, "invalid bookmark id"),
			Entry("zero", "/bookmarks/0", http.StatusBadRequest, "invalid bookmark id"),
			Entry("missing", "/bookmarks/"+uuid.NewString(), http.StatusNotFound, "bookmark not found"),
		)
	})
})
