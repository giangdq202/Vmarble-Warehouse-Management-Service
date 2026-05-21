package httpkit

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// ── ValidateOffset ───────────────────────────────────────────────────────────

func TestValidateOffset_UnderCap_Allowed(t *testing.T) {
	cases := []PageParams{
		{Page: 1, Limit: 10},      // offset 0
		{Page: 1000, Limit: 10},   // offset 9990
		{Page: 100, Limit: 100},   // offset 9900
		{Page: 1, Limit: 100},     // offset 0
	}
	for _, p := range cases {
		if err := ValidateOffset(p); err != nil {
			t.Errorf("ValidateOffset(page=%d, limit=%d) = %v, want nil (offset %d <= cap)", p.Page, p.Limit, err, p.Offset())
		}
	}
}

func TestValidateOffset_AtCap_Allowed(t *testing.T) {
	// page=1001, limit=10 → offset 10000 — exactly at cap, must pass.
	p := PageParams{Page: 1001, Limit: 10}
	if got := p.Offset(); got != MaxOffset {
		t.Fatalf("test setup: offset = %d, want %d", got, MaxOffset)
	}
	if err := ValidateOffset(p); err != nil {
		t.Errorf("ValidateOffset at cap = %v, want nil (cap is inclusive)", err)
	}
}

func TestValidateOffset_OverCap_Rejected(t *testing.T) {
	cases := []PageParams{
		{Page: 1002, Limit: 10},  // offset 10010
		{Page: 102, Limit: 100},  // offset 10100
		{Page: 999999, Limit: 1}, // pathological deep page
	}
	for _, p := range cases {
		err := ValidateOffset(p)
		if err == nil {
			t.Errorf("ValidateOffset(page=%d, limit=%d) = nil, want error (offset %d > cap)", p.Page, p.Limit, p.Offset())
			continue
		}
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("err = %v, want wraps ErrInvalidInput so httpkit.Error maps to 400", err)
		}
		// Hint should name the alternative path so callers know what to do.
		if !strings.Contains(err.Error(), "date filter") && !strings.Contains(err.Error(), "cursor") {
			t.Errorf("err = %q, want it to mention 'date filter' or 'cursor' so the hint is actionable", err)
		}
	}
}

// ── NewPagedResultEstimated ──────────────────────────────────────────────────

func TestNewPagedResultEstimated_FlagsTotalAsEstimate(t *testing.T) {
	got := NewPagedResultEstimated([]int{1, 2, 3}, 1_234_000, PageParams{Page: 1, Limit: 10})
	if !got.TotalIsEstimate {
		t.Error("TotalIsEstimate = false, want true")
	}
	if got.TotalItems != 1_234_000 {
		t.Errorf("TotalItems = %d, want 1234000", got.TotalItems)
	}
}

func TestNewPagedResult_OmitsEstimateFlagWhenFalse(t *testing.T) {
	got := NewPagedResult([]int{1}, 1, PageParams{Page: 1, Limit: 10})
	if got.TotalIsEstimate {
		t.Error("TotalIsEstimate = true, want false (default constructor)")
	}
}

// ── ValidateOffset routes through httpkit.Error → 400 ────────────────────────

func TestValidateOffset_TranslatesTo400_ViaHttpkitError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := ValidateOffset(PageParams{Page: 5000, Limit: 100}) // offset 499900
	if err == nil {
		t.Fatal("test setup: ValidateOffset returned nil, want error")
	}
	Error(c, err)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400 (offset cap → bad request)", w.Code)
	}
}
