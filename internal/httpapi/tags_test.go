package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// tagFormatError is the message a tag that breaks the alphabet or length rule is answered with.
	tagFormatError = "tag must be 1 to 30 characters from a-z, 0-9 and -"
	// tooManyTagsError is the message a request carrying more than maxTagsPerBookmark tags gets.
	tooManyTagsError = "a bookmark has at most 10 tags"
	// tagsRequiredError is the message a body without a tags array is answered with.
	tagsRequiredError = "tags is required"
)

// rawTags returns the raw JSON value of a bookmark response's tags field, so a spec can tell an
// empty array apart from null.
func rawTags(body []byte) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	return fields["tags"], nil
}

// bookmarkTagsOf decodes the tags array of a bookmark response body.
func bookmarkTagsOf(body []byte) []string {
	raw, err := rawTags(body)
	Expect(err).NotTo(HaveOccurred())
	var tags []string
	Expect(json.Unmarshal(raw, &tags)).To(Succeed())
	return tags
}

// tagsBody builds a PUT /bookmarks/{id}/tags body from tags.
func tagsBody(tags ...string) string {
	encoded, _ := json.Marshal(map[string][]string{"tags": tags})
	return string(encoded)
}

// tagNames returns n distinct valid tag names: tag-0, tag-1 and so on, already ascending.
func tagNames(n int) []string {
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		names = append(names, fmt.Sprintf("tag-%d", i))
	}
	return names
}

var _ = Describe("PUT /bookmarks/{id}/tags", func() {
	var router http.Handler

	BeforeEach(func() {
		router = newTestRouter()
		Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://go.dev","title":"Go"}`).Code).To(Equal(http.StatusCreated))
	})

	It("replaces the tag set and answers with the bookmark", func() {
		rec := do(router, http.MethodPut, "/bookmarks/1/tags", `{"tags":["go","chi"]}`)
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())

		var replaced bookmarkResponse
		Expect(json.Unmarshal(rec.Body.Bytes(), &replaced)).To(Succeed())
		Expect(replaced.ID).To(BeEquivalentTo(1))
		Expect(replaced.URL).To(Equal("https://go.dev"))
		Expect(replaced.Title).To(Equal("Go"))
		Expect(replaced.CreatedAt.IsZero()).To(BeFalse())
		Expect(replaced.Tags).To(Equal([]string{"chi", "go"}))

		rec = do(router, http.MethodGet, "/bookmarks/1", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal([]string{"chi", "go"}))
	})

	It("replaces instead of adding on a second call", func() {
		Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go", "chi")).Code).To(Equal(http.StatusOK))
		Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go")).Code).To(Equal(http.StatusOK))

		rec := do(router, http.MethodGet, "/bookmarks/1", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal([]string{"go"}))
	})

	It("clears every tag when the set is empty", func() {
		Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go")).Code).To(Equal(http.StatusOK))

		rec := do(router, http.MethodPut, "/bookmarks/1/tags", `{"tags":[]}`)
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
		raw, err := rawTags(rec.Body.Bytes())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(Equal("[]"))
	})

	It("counts a duplicate once", func() {
		rec := do(router, http.MethodPut, "/bookmarks/1/tags", `{"tags":["go","go"]}`)
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal([]string{"go"}))
	})

	It("accepts exactly 10 distinct tags", func() {
		rec := do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody(tagNames(10)...))
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal(tagNames(10)))
	})

	DescribeTable("rejects a body",
		func(body string, status int, message string) {
			rec := do(router, http.MethodPut, "/bookmarks/1/tags", body)
			Expect(rec.Code).To(Equal(status), rec.Body.String())
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
		},
		Entry("no tags key", `{}`, http.StatusBadRequest, tagsRequiredError),
		Entry("null tags", `{"tags":null}`, http.StatusBadRequest, tagsRequiredError),
		Entry("malformed JSON", `{"tags":`, http.StatusBadRequest, "invalid JSON body"),
		Entry("capital letter", tagsBody("Go"), http.StatusBadRequest, tagFormatError),
		Entry("underscore", tagsBody("go_lang"), http.StatusBadRequest, tagFormatError),
		Entry("empty name", tagsBody(""), http.StatusBadRequest, tagFormatError),
		Entry("31 characters", tagsBody(strings.Repeat("a", 31)), http.StatusBadRequest, tagFormatError),
		Entry("too many tags", tagsBody(tagNames(11)...), http.StatusBadRequest, tooManyTagsError),
		Entry("11 tags including a bad name", tagsBody(append([]string{"Go"}, tagNames(11)...)...), http.StatusBadRequest, tagFormatError),
	)

	It("leaves the previous tags alone when the request is rejected", func() {
		Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go", "chi")).Code).To(Equal(http.StatusOK))

		rec := do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("chi", "Go"))
		Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
		Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + tagFormatError + `"}`))

		rec = do(router, http.MethodGet, "/bookmarks/1", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal([]string{"chi", "go"}))
	})

	DescribeTable("answers for a bad id or a missing bookmark",
		func(target, body string, status int, message string) {
			rec := do(router, http.MethodPut, target, body)
			Expect(rec.Code).To(Equal(status), rec.Body.String())
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
		},
		Entry("not a number", "/bookmarks/abc/tags", tagsBody("go"), http.StatusBadRequest, "invalid bookmark id"),
		Entry("zero", "/bookmarks/0/tags", tagsBody("go"), http.StatusBadRequest, "invalid bookmark id"),
		Entry("missing", "/bookmarks/99/tags", tagsBody("go"), http.StatusNotFound, "bookmark not found"),
		Entry("missing, invalid body", "/bookmarks/99/tags", `{"tags":null}`, http.StatusNotFound, "bookmark not found"),
		Entry("missing, malformed JSON", "/bookmarks/99/tags", `{"tags":`, http.StatusNotFound, "bookmark not found"),
	)
})

var _ = Describe("GET /bookmarks with a tag filter", func() {
	var router http.Handler

	BeforeEach(func() {
		router = newTestRouter()

		Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://a.example","title":"first"}`).Code).To(Equal(http.StatusCreated))
		Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://b.example","title":"second"}`).Code).To(Equal(http.StatusCreated))
		Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://c.example","title":"third"}`).Code).To(Equal(http.StatusCreated))

		Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go", "chi")).Code).To(Equal(http.StatusOK))
		Expect(do(router, http.MethodPut, "/bookmarks/2/tags", tagsBody("chi")).Code).To(Equal(http.StatusOK))
		Expect(do(router, http.MethodPut, "/bookmarks/3/tags", tagsBody("go")).Code).To(Equal(http.StatusOK))
	})

	It("lists only the bookmarks carrying the tag, oldest first, with all their tags", func() {
		rec := do(router, http.MethodGet, "/bookmarks?tag=go", "")
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())

		var bookmarks []bookmarkResponse
		Expect(json.Unmarshal(rec.Body.Bytes(), &bookmarks)).To(Succeed())
		Expect(bookmarks).To(HaveLen(2))
		Expect(bookmarks[0].Title).To(Equal("first"))
		Expect(bookmarks[0].Tags).To(Equal([]string{"chi", "go"}))
		Expect(bookmarks[1].Title).To(Equal("third"))
		Expect(bookmarks[1].Tags).To(Equal([]string{"go"}))
	})

	It("still lists every bookmark when no tag is given", func() {
		rec := do(router, http.MethodGet, "/bookmarks", "")
		Expect(rec.Code).To(Equal(http.StatusOK))

		var bookmarks []bookmarkResponse
		Expect(json.Unmarshal(rec.Body.Bytes(), &bookmarks)).To(Succeed())
		Expect(bookmarks).To(HaveLen(3))
		Expect(bookmarks[1].Title).To(Equal("second"))
		Expect(bookmarks[1].Tags).To(Equal([]string{"chi"}))
	})

	It("is [] for a tag nothing carries", func() {
		rec := do(router, http.MethodGet, "/bookmarks?tag=nosuchtag", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(MatchJSON(`[]`))
	})

	DescribeTable("is [] for a value that is not a tag",
		func(value string) {
			rec := do(router, http.MethodGet, "/bookmarks?tag="+value, "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(MatchJSON(`[]`))
		},
		Entry("capitals", "Go"),
		Entry("empty", ""),
		Entry("31 characters", strings.Repeat("a", 31)),
		Entry("underscore", "go_lang"),
	)
})

var _ = Describe("the tags field on bookmark responses", func() {
	var router http.Handler

	BeforeEach(func() {
		router = newTestRouter()
		Expect(do(router, http.MethodPost, "/bookmarks", `{"url":"https://go.dev","title":"Go"}`).Code).To(Equal(http.StatusCreated))
	})

	It("is the raw JSON [] and never null on a bookmark with no tags", func() {
		rec := do(router, http.MethodGet, "/bookmarks/1", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		raw, err := rawTags(rec.Body.Bytes())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(Equal("[]"))

		rec = do(router, http.MethodGet, "/bookmarks", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		var listed []map[string]json.RawMessage
		Expect(json.Unmarshal(rec.Body.Bytes(), &listed)).To(Succeed())
		Expect(listed).To(HaveLen(1))
		Expect(string(listed[0]["tags"])).To(Equal("[]"))
	})

	It("is [] on a bookmark created with a tags key in the body, which is ignored", func() {
		rec := do(router, http.MethodPost, "/bookmarks", `{"url":"https://example.com","title":"Example","tags":["ignored"]}`)
		Expect(rec.Code).To(Equal(http.StatusCreated), rec.Body.String())
		raw, err := rawTags(rec.Body.Bytes())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(Equal("[]"))
	})

	It("is a sorted array once tags are set", func() {
		Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go", "chi", "bookmarks")).Code).To(Equal(http.StatusOK))

		rec := do(router, http.MethodGet, "/bookmarks/1", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal([]string{"bookmarks", "chi", "go"}))
	})

	It("is carried by PUT /bookmarks/{id} and unchanged by a tags key in its body", func() {
		Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go")).Code).To(Equal(http.StatusOK))

		rec := do(router, http.MethodPut, "/bookmarks/1", `{"url":"https://go.dev/blog","title":"The Go Blog","tags":["ignored"]}`)
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal([]string{"go"}))

		rec = do(router, http.MethodGet, "/bookmarks/1", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(bookmarkTagsOf(rec.Body.Bytes())).To(Equal([]string{"go"}))
	})
})
