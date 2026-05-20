package httpkit

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Cursor is the keyset pagination state — the (sort_ts, id) pair of the last
// row the client received. The storage layer compares incoming rows against
// this pair to skip past it without scanning skipped pages.
//
// Why timestamp + uuid (not timestamp alone)? Timestamps collide regularly —
// two scan_events written in the same millisecond is normal. Without the
// tie-break ID, a row at the boundary can either be returned twice or skipped
// entirely depending on the order Postgres happens to return rows.
//
// Why not opaque integer offsets? See Markus Winand's "no-offset paging" —
// keyset stays O(log N) on a btree index regardless of how deep the user has
// paged, while OFFSET stays O(N+offset) and OOMs predictably.
type Cursor struct {
	Ts time.Time `json:"ts"`
	ID uuid.UUID `json:"id"`
}

// IsZero reports whether the cursor is the zero value (i.e. caller did not
// pass one — first page).
func (c Cursor) IsZero() bool {
	return c.Ts.IsZero() && c.ID == uuid.Nil
}

// Encode renders the cursor as base64url(JSON). The opaque envelope means
// clients must treat it as a token they hand back unmodified — they cannot
// hand-craft a cursor pointing into the middle of someone else's data.
//
// JSON (not a packed binary form) is intentional: future fields can be added
// without bumping a version, and an operator decoding a cursor in a bug
// report can read it.
func (c Cursor) Encode() string {
	if c.IsZero() {
		return ""
	}
	raw, _ := json.Marshal(c) // never fails for this struct
	return base64.RawURLEncoding.EncodeToString(raw)
}

// ErrInvalidCursor is returned when a client-supplied cursor cannot be
// decoded. Handlers should translate this to 400 Bad Request, not 500 —
// the bad token is the caller's fault.
var ErrInvalidCursor = errors.New("invalid cursor")

// DecodeCursor parses an opaque cursor string. Empty input returns the zero
// Cursor with no error so the "first page" case stays cheap.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: bad base64", ErrInvalidCursor)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, fmt.Errorf("%w: bad json", ErrInvalidCursor)
	}
	if c.IsZero() {
		return Cursor{}, fmt.Errorf("%w: empty cursor", ErrInvalidCursor)
	}
	return c, nil
}

// CursorParams holds validated keyset pagination inputs. Handlers bind these
// from query strings using BindCursorParams.
type CursorParams struct {
	Cursor string // raw, opaque token; call Decoded() to parse
	Limit  int    // 1..maxPageLimit; defaults to defaultPageLimit
}

// Decoded parses the cursor token. Callers should propagate ErrInvalidCursor
// to the client as 400.
func (p CursorParams) Decoded() (Cursor, error) {
	return DecodeCursor(p.Cursor)
}

// BindCursorParams reads cursor and limit from the request query string.
// Limit defaults and cap match the existing offset-based PageParams so
// migration between the two does not surprise FE consumers.
func BindCursorParams(c *gin.Context) CursorParams {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultPageLimit)))
	if limit < 1 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	return CursorParams{
		Cursor: c.Query("cursor"),
		Limit:  limit,
	}
}

// CursorResult is the standard envelope returned by keyset-paginated list
// endpoints. NextCursor is omitted (not just empty) when there are no more
// rows so clients can use plain "if has next_cursor" branching.
type CursorResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// NewCursorResult builds the response envelope from an over-fetched slice.
// Stores fetch limit+1 rows so this helper can detect "has next page"
// without an extra round-trip; callers pass the over-fetched slice and a
// cursor extractor for the row type.
//
// Wire pattern:
//
//	rows, err := store.ListKeyset(ctx, params.Limit+1, cur)
//	...
//	return httpkit.NewCursorResult(rows, params.Limit, func(r Row) Cursor {
//	    return Cursor{Ts: r.CreatedAt, ID: r.ID}
//	})
//
// The store's responsibility ends at "give me up to N+1 rows in the right
// order"; pagination math lives here.
func NewCursorResult[T any](items []T, limit int, getCursor func(T) Cursor) CursorResult[T] {
	if items == nil {
		items = []T{}
	}
	if limit < 1 {
		limit = defaultPageLimit
	}
	if len(items) <= limit {
		return CursorResult[T]{Items: items, HasMore: false}
	}
	kept := items[:limit]
	return CursorResult[T]{
		Items:      kept,
		HasMore:    true,
		NextCursor: getCursor(kept[len(kept)-1]).Encode(),
	}
}
