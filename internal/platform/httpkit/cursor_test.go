package httpkit

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestCursor_EncodeDecode_RoundTrip(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	ts := time.Date(2026, 5, 20, 10, 30, 45, 123_000_000, time.UTC)

	original := Cursor{Ts: ts, ID: id}
	encoded := original.Encode()

	if encoded == "" {
		t.Fatal("Encode() returned empty for non-zero cursor")
	}

	got, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor(%q) = %v", encoded, err)
	}
	if !got.Ts.Equal(original.Ts) {
		t.Errorf("Ts = %v, want %v", got.Ts, original.Ts)
	}
	if got.ID != original.ID {
		t.Errorf("ID = %v, want %v", got.ID, original.ID)
	}
}

func TestCursor_EncodeZero_ReturnsEmpty(t *testing.T) {
	if got := (Cursor{}).Encode(); got != "" {
		t.Errorf("zero Cursor Encode() = %q, want empty", got)
	}
}

func TestDecodeCursor_EmptyInput_ReturnsZero(t *testing.T) {
	c, err := DecodeCursor("")
	if err != nil {
		t.Fatalf("DecodeCursor(\"\") = %v, want nil error", err)
	}
	if !c.IsZero() {
		t.Errorf("DecodeCursor(\"\") = %+v, want zero cursor", c)
	}
}

func TestDecodeCursor_BadInput_ReturnsErrInvalidCursor(t *testing.T) {
	cases := map[string]string{
		"not-base64":  "not!valid!base64!@#",
		"base64-not-json": "Zm9vYmFy",                                                    // "foobar"
		"json-but-empty": "e30",                                                          // "{}" → IsZero
		"json-zero-id":   "eyJ0cyI6IjAwMDEtMDEtMDFUMDA6MDA6MDBaIiwiaWQiOiIifQ",           // {"ts":"..","id":""}
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeCursor(in)
			if err == nil {
				t.Errorf("DecodeCursor(%q) = nil error, want ErrInvalidCursor", in)
				return
			}
			if !strings.Contains(err.Error(), "invalid cursor") {
				t.Errorf("err = %q, want it to wrap ErrInvalidCursor", err)
			}
		})
	}
}

func TestBindCursorParams_Defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x", nil)

	p := BindCursorParams(c)
	if p.Limit != defaultPageLimit {
		t.Errorf("Limit = %d, want %d", p.Limit, defaultPageLimit)
	}
	if p.Cursor != "" {
		t.Errorf("Cursor = %q, want empty", p.Cursor)
	}
}

func TestBindCursorParams_LimitClampedToMax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?limit=10000", nil)

	if got := BindCursorParams(c).Limit; got != maxPageLimit {
		t.Errorf("Limit = %d, want capped at %d", got, maxPageLimit)
	}
}

func TestBindCursorParams_NegativeLimitDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?limit=-5", nil)

	if got := BindCursorParams(c).Limit; got != defaultPageLimit {
		t.Errorf("Limit = %d, want %d (negative falls back to default)", got, defaultPageLimit)
	}
}

func TestBindCursorParams_PassesThroughCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	token := Cursor{Ts: time.Now().UTC(), ID: uuid.New()}.Encode()
	c.Request = httptest.NewRequest("GET", "/x?cursor="+url.QueryEscape(token), nil)

	if got := BindCursorParams(c).Cursor; got != token {
		t.Errorf("Cursor = %q, want %q", got, token)
	}
}

type row struct {
	id uuid.UUID
	ts time.Time
}

func rowCursor(r row) Cursor { return Cursor{Ts: r.ts, ID: r.id} }

func TestNewCursorResult_FewerThanLimit_HasNoMore(t *testing.T) {
	rows := []row{
		{id: uuid.New(), ts: time.Now()},
		{id: uuid.New(), ts: time.Now()},
	}
	got := NewCursorResult(rows, 10, rowCursor)
	if got.HasMore {
		t.Errorf("HasMore = true, want false (returned %d rows < limit 10)", len(rows))
	}
	if got.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty", got.NextCursor)
	}
	if len(got.Items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(got.Items))
	}
}

func TestNewCursorResult_OverFetched_TrimsAndSetsCursor(t *testing.T) {
	limit := 3
	rows := []row{
		{id: uuid.New(), ts: time.Unix(1, 0)},
		{id: uuid.New(), ts: time.Unix(2, 0)},
		{id: uuid.New(), ts: time.Unix(3, 0)}, // last kept; cursor should encode this
		{id: uuid.New(), ts: time.Unix(4, 0)}, // sentinel proving HasMore
	}
	got := NewCursorResult(rows, limit, rowCursor)
	if !got.HasMore {
		t.Errorf("HasMore = false, want true (over-fetched %d > limit %d)", len(rows), limit)
	}
	if len(got.Items) != limit {
		t.Errorf("len(items) = %d, want %d (sentinel must be trimmed)", len(got.Items), limit)
	}
	if got.NextCursor == "" {
		t.Fatal("NextCursor empty, want non-empty")
	}
	decoded, err := DecodeCursor(got.NextCursor)
	if err != nil {
		t.Fatalf("DecodeCursor(NextCursor) = %v", err)
	}
	wantTs := time.Unix(3, 0)
	if !decoded.Ts.Equal(wantTs) {
		t.Errorf("cursor.Ts = %v, want %v (last kept row, not sentinel)", decoded.Ts, wantTs)
	}
}

func TestNewCursorResult_NilSlice_ReturnsEmptyArrayNotNil(t *testing.T) {
	got := NewCursorResult[row](nil, 10, rowCursor)
	if got.Items == nil {
		t.Error("Items is nil; want empty slice so JSON renders [] not null")
	}
}
