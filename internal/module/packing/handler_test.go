package packing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type stubPackingService struct{}

func (stubPackingService) CreateFromCompletedWO(context.Context, CreateFromCompletedWOInput) ([]FGPool, error) {
	panic("unexpected")
}
func (stubPackingService) GetFG(context.Context, uuid.UUID) (FGPool, error) { panic("unexpected") }
func (stubPackingService) ListFG(_ context.Context, _ httpkit.PageParams, _ FGListFilter) (httpkit.PagedResult[FGPool], error) {
	return httpkit.PagedResult[FGPool]{Items: []FGPool{}, TotalItems: 0, TotalPages: 1, CurrentPage: 1, Limit: 10}, nil
}
func (stubPackingService) ScanBarcode(context.Context, uuid.UUID, uuid.UUID) (ScanResult, error) {
	panic("unexpected")
}
func (stubPackingService) ReportDefect(context.Context, ReportDefectInput) (DefectReportResult, error) {
	panic("unexpected")
}
func (stubPackingService) ResolveDefect(context.Context, ResolveDefectInput) (FGDefect, error) {
	panic("unexpected")
}
func (stubPackingService) ReserveOnContainerAdd(context.Context, ReserveInput) (int, error) {
	panic("unexpected")
}
func (stubPackingService) ReleaseOnContainerDelete(context.Context, uuid.UUID) error {
	panic("unexpected")
}
func (stubPackingService) MarkLoadedOnSeal(context.Context, uuid.UUID) error {
	panic("unexpected")
}
func (stubPackingService) CheckComponentsForSeal(context.Context, uuid.UUID) error {
	panic("unexpected")
}
func (stubPackingService) ReassignFG(context.Context, ReassignFGInput) (ReassignFGResult, error) {
	panic("unexpected")
}

var _ Service = stubPackingService{}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(stubPackingService{})
	r := gin.New()
	h.Register(r.Group("/api/v1"))
	return r
}

func TestListFGPool_NoFilter_Returns200(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fg-pool", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestListFGPool_ValidDateRange_Returns200(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fg-pool?from=2026-05-01&to=2026-06-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListFGPool_OnlyFrom_Returns400(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fg-pool?from=2026-05-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when only from provided, got %d", w.Code)
	}
}

func TestListFGPool_OnlyTo_Returns400(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fg-pool?to=2026-06-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when only to provided, got %d", w.Code)
	}
}

func TestListFGPool_FromAfterTo_Returns400(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fg-pool?from=2026-06-01&to=2026-05-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when from > to, got %d", w.Code)
	}
}

func TestListFGPool_InvalidFromFormat_Returns400(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fg-pool?from=not-a-date&to=2026-06-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid from date, got %d", w.Code)
	}
}

func TestListFGPool_RFC3339DateRange_Returns200(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fg-pool?from=2026-05-01T00:00:00Z&to=2026-06-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for RFC3339 range, got %d", w.Code)
	}
}
