package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// totalCount returns the number in the X-Total-Count header of rec, failing the spec when the
// header is absent or does not hold a number.
func totalCount(rec *httptest.ResponseRecorder) int {
	raw := rec.Header().Get(totalCountHeader)
	Expect(raw).NotTo(BeEmpty(), "X-Total-Count header is missing")
	total, err := strconv.Atoi(raw)
	Expect(err).NotTo(HaveOccurred(), "X-Total-Count is not a number: %q", raw)
	return total
}

// listedIDs returns the ids of the bookmarks in a listing response body, in the order they appear.
func listedIDs(body []byte) []int64 {
	var bookmarks []bookmarkResponse
	Expect(json.Unmarshal(body, &bookmarks)).To(Succeed())
	ids := make([]int64, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		ids = append(ids, bookmark.ID)
	}
	return ids
}

// seedBookmarks creates one bookmark per title, in order, and fails the spec if any is rejected.
// Every bookmark gets its own valid url, so the titles are the only part the caller cares about.
func seedBookmarks(router http.Handler, titles ...string) {
	for i, title := range titles {
		body, err := json.Marshal(map[string]string{
			"url":   fmt.Sprintf("https://example.com/%d", i+1),
			"title": title,
		})
		Expect(err).NotTo(HaveOccurred())
		rec := do(router, http.MethodPost, "/bookmarks", string(body))
		Expect(rec.Code).To(Equal(http.StatusCreated), rec.Body.String())
	}
}

var _ = Describe("GET /bookmarks paging, sorting and X-Total-Count", func() {
	var router http.Handler

	BeforeEach(func() {
		router = newTestRouter()
	})

	Describe("the default page", func() {
		It("answers 20 bookmarks in id order and the total before paging", func() {
			titles := make([]string, 0, 25)
			for i := 1; i <= 25; i++ {
				titles = append(titles, fmt.Sprintf("bookmark-%02d", i))
			}
			seedBookmarks(router, titles...)

			rec := do(router, http.MethodGet, "/bookmarks", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(totalCount(rec)).To(Equal(25))

			want := make([]int64, 0, 20)
			for id := int64(1); id <= 20; id++ {
				want = append(want, id)
			}
			Expect(listedIDs(rec.Body.Bytes())).To(Equal(want))
		})
	})

	Describe("limit", func() {
		BeforeEach(func() {
			seedBookmarks(router, "first", "second", "third")
		})

		DescribeTable("pages the listing and leaves the total alone",
			func(query string, wantIDs []int64) {
				rec := do(router, http.MethodGet, "/bookmarks?"+query, "")
				Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
				Expect(listedIDs(rec.Body.Bytes())).To(Equal(wantIDs))
				Expect(totalCount(rec)).To(Equal(3))
			},
			Entry("limit 1", "limit=1", []int64{1}),
			Entry("limit 100 is the largest accepted", "limit=100", []int64{1, 2, 3}),
		)

		DescribeTable("rejects a value outside 1 to 100",
			func(query string) {
				rec := do(router, http.MethodGet, "/bookmarks?"+query, "")
				Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"invalid limit"}`))
			},
			Entry("zero", "limit=0"),
			Entry("above the maximum", "limit=101"),
			Entry("negative", "limit=-1"),
			Entry("not a number", "limit=abc"),
			Entry("empty", "limit="),
		)
	})

	Describe("offset", func() {
		BeforeEach(func() {
			seedBookmarks(router, "first", "second", "third")
		})

		It("pages past the first bookmarks", func() {
			rec := do(router, http.MethodGet, "/bookmarks?limit=2&offset=1", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{2, 3}))
			Expect(totalCount(rec)).To(Equal(3))
		})

		It("answers [] past the end with the full total, not an error", func() {
			rec := do(router, http.MethodGet, "/bookmarks?offset=10", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(rec.Body.String()).To(MatchJSON(`[]`))
			Expect(totalCount(rec)).To(Equal(3))
		})

		DescribeTable("rejects a negative or non-numeric value",
			func(query string) {
				rec := do(router, http.MethodGet, "/bookmarks?"+query, "")
				Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
				Expect(rec.Body.String()).To(MatchJSON(`{"error":"invalid offset"}`))
			},
			Entry("negative", "offset=-1"),
			Entry("not a number", "offset=abc"),
			Entry("empty", "offset="),
		)
	})

	Describe("sort by title", func() {
		BeforeEach(func() {
			// Zebra sorts before apple in SQLite's default BINARY collation, which is what the
			// ticket asks for: byte order, not a case-insensitive one.
			seedBookmarks(router, "apple", "Zebra")
		})

		It("orders titles case-sensitively", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=title", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{2, 1}))
		})

		It("reverses that order for order=desc", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=title&order=desc", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{1, 2}))
		})
	})

	Describe("sort by title with a shared title", func() {
		BeforeEach(func() {
			seedBookmarks(router, "same", "same", "same")
		})

		It("breaks the tie on id ascending", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=title", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{1, 2, 3}))
		})

		It("breaks the tie on id descending", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=title&order=desc", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{3, 2, 1}))
		})
	})

	Describe("sort by created_at", func() {
		BeforeEach(func() {
			seedBookmarks(router, "first", "second", "third")
		})

		It("answers insertion order", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=created_at", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{1, 2, 3}))
			Expect(totalCount(rec)).To(Equal(3))
		})

		It("answers the reverse for order=desc", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=created_at&order=desc", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{3, 2, 1}))
			Expect(totalCount(rec)).To(Equal(3))
		})
	})

	Describe("sort by created_at with one timestamp on every bookmark", func() {
		var db *sql.DB

		BeforeEach(func() {
			// The API never lets a client set created_at, so two rows can only share a timestamp
			// when a spec writes it straight to the database; the tie-break on id is otherwise
			// unreachable over HTTP.
			router, db = newTestRouterWithDB()
			seedBookmarks(router, "first", "second", "third")

			_, err := db.Exec("UPDATE bookmarks SET created_at = (SELECT created_at FROM bookmarks WHERE id = 1)")
			Expect(err).NotTo(HaveOccurred())
		})

		It("breaks the tie on id ascending", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=created_at", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{1, 2, 3}))
		})

		It("breaks the tie on id descending", func() {
			rec := do(router, http.MethodGet, "/bookmarks?sort=created_at&order=desc", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{3, 2, 1}))
		})
	})

	DescribeTable("rejects an unusable sort or order",
		func(query, message string) {
			rec := do(router, http.MethodGet, "/bookmarks?"+query, "")
			Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"` + message + `"}`))
		},
		Entry("sort by url", "sort=url", "invalid sort"),
		Entry("sort by name", "sort=name", "invalid sort"),
		Entry("sort with capitals", "sort=ID", "invalid sort"),
		Entry("sort empty", "sort=", "invalid sort"),
		Entry("order up", "order=up", "invalid order"),
		Entry("order with capitals", "order=ASC", "invalid order"),
		Entry("order empty", "order=", "invalid order"),
		Entry("the earliest bad parameter wins", "limit=abc&sort=url", "invalid limit"),
		Entry("a bad offset beats a bad order", "offset=abc&order=up", "invalid offset"),
	)

	Describe("with a tag that is not a tag", func() {
		BeforeEach(func() {
			seedBookmarks(router, "first")
		})

		It("answers 400 when a paging parameter is invalid too, since the 400 wins", func() {
			rec := do(router, http.MethodGet, "/bookmarks?tag=BAD&limit=abc", "")
			Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
			Expect(rec.Body.String()).To(MatchJSON(`{"error":"invalid limit"}`))
		})

		It("answers [] with a total of zero", func() {
			rec := do(router, http.MethodGet, "/bookmarks?tag=BAD", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(rec.Body.String()).To(MatchJSON(`[]`))
			Expect(totalCount(rec)).To(Equal(0))
		})
	})

	Describe("with a tag filter", func() {
		BeforeEach(func() {
			seedBookmarks(router, "first", "second", "third")
			Expect(do(router, http.MethodPut, "/bookmarks/1/tags", tagsBody("go")).Code).To(Equal(http.StatusOK))
			Expect(do(router, http.MethodPut, "/bookmarks/2/tags", tagsBody("go")).Code).To(Equal(http.StatusOK))
			Expect(do(router, http.MethodPut, "/bookmarks/3/tags", tagsBody("chi")).Code).To(Equal(http.StatusOK))
		})

		It("pages and counts only the bookmarks carrying the tag", func() {
			rec := do(router, http.MethodGet, "/bookmarks?tag=go&limit=1&offset=1", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{2}))
			Expect(totalCount(rec)).To(Equal(2))
		})

		It("sorts only the bookmarks carrying the tag", func() {
			rec := do(router, http.MethodGet, "/bookmarks?tag=go&sort=id&order=desc", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(listedIDs(rec.Body.Bytes())).To(Equal([]int64{2, 1}))
			Expect(totalCount(rec)).To(Equal(2))
		})

		It("answers [] with the full total past the end", func() {
			rec := do(router, http.MethodGet, "/bookmarks?tag=go&offset=5", "")
			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
			Expect(rec.Body.String()).To(MatchJSON(`[]`))
			Expect(totalCount(rec)).To(Equal(2))
		})
	})
})
