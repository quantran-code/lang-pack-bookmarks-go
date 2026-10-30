package httpapi

import (
	"errors"
	"net/http"
	"strconv"
)

const (
	// defaultLimit is how many bookmarks a listing carries when ?limit= is absent.
	defaultLimit = 20
	// maxLimit is the largest limit a listing accepts.
	maxLimit = 100
)

// sortField is the bookmark column a listing is sorted on.
type sortField string

const (
	// sortByID sorts by the bookmark's UUID id, compared as text.
	sortByID sortField = "id"
	// sortByTitle sorts by title, compared with SQLite's default BINARY collation.
	sortByTitle sortField = "title"
	// sortByCreatedAt sorts by the creation timestamp, the default.
	sortByCreatedAt sortField = "created_at"
)

// sortOrder is the direction a listing is sorted in.
type sortOrder string

const (
	// orderAsc is ascending, the default.
	orderAsc sortOrder = "asc"
	// orderDesc is descending.
	orderDesc sortOrder = "desc"
)

var (
	// errInvalidLimit is the message a limit outside 1 to maxLimit is answered with.
	errInvalidLimit = errors.New("invalid limit")
	// errInvalidOffset is the message a negative or non-numeric offset is answered with.
	errInvalidOffset = errors.New("invalid offset")
	// errInvalidSort is the message a sort field other than id, title and created_at is answered with.
	errInvalidSort = errors.New("invalid sort")
	// errInvalidOrder is the message an order other than asc and desc is answered with.
	errInvalidOrder = errors.New("invalid order")
)

// listParams is a listing request after parsing: every field holds either the value from the query
// string or its default, never a zero value left behind by omission.
type listParams struct {
	Limit  int
	Offset int
	Sort   sortField
	Order  sortOrder
}

// parseListParams reads limit, offset, sort and order from the query string. The parameters are
// checked in that order and the first failure is returned, so a request that breaks several rules
// is answered with the message for the earliest one. A key that is present with an empty value is
// rejected rather than treated as absent, because an empty value is not one of the accepted ones.
func parseListParams(r *http.Request) (listParams, error) {
	query := r.URL.Query()
	params := listParams{
		Limit:  defaultLimit,
		Offset: 0,
		Sort:   sortByCreatedAt,
		Order:  orderAsc,
	}

	if query.Has("limit") {
		limit, err := strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > maxLimit {
			return listParams{}, errInvalidLimit
		}
		params.Limit = limit
	}

	if query.Has("offset") {
		offset, err := strconv.Atoi(query.Get("offset"))
		if err != nil || offset < 0 {
			return listParams{}, errInvalidOffset
		}
		params.Offset = offset
	}

	if query.Has("sort") {
		switch field := sortField(query.Get("sort")); field {
		case sortByID, sortByTitle, sortByCreatedAt:
			params.Sort = field
		default:
			return listParams{}, errInvalidSort
		}
	}

	if query.Has("order") {
		switch direction := sortOrder(query.Get("order")); direction {
		case orderAsc, orderDesc:
			params.Order = direction
		default:
			return listParams{}, errInvalidOrder
		}
	}

	return params, nil
}
