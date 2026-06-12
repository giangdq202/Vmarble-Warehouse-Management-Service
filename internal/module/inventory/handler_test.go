package inventory

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type stubInventoryService struct{}

func (stubInventoryService) ReceiveStock(context.Context, ReceiveStockInput) (InventoryLot, error) {
	panic("unexpected")
}
func (stubInventoryService) ListLots(context.Context, httpkit.CursorParams, string) (httpkit.CursorResult[InventoryLot], error) {
	panic("unexpected")
}
func (stubInventoryService) DeactivateLot(context.Context, uuid.UUID) error { panic("unexpected") }
func (stubInventoryService) ExportLots(context.Context, httpkit.PageParams, io.Writer) error {
	panic("unexpected")
}
func (stubInventoryService) QCPassLot(context.Context, uuid.UUID, uuid.UUID) error {
	panic("unexpected")
}
func (stubInventoryService) RejectLot(context.Context, RejectLotInput) (RejectLotResult, error) {
	panic("unexpected")
}
func (stubInventoryService) ListRejections(_ context.Context, _ RejectionFilter, _ httpkit.CursorParams) (httpkit.CursorResult[MaterialRejection], error) {
	return httpkit.CursorResult[MaterialRejection]{Items: []MaterialRejection{}}, nil
}
func (stubInventoryService) GetRejection(context.Context, uuid.UUID) (MaterialRejection, error) {
	panic("unexpected")
}
func (stubInventoryService) UpdateRejectionClaim(context.Context, UpdateClaimInput) (MaterialRejection, error) {
	panic("unexpected")
}
func (stubInventoryService) RejectionReport(context.Context, RejectionReportFilter) ([]RejectionReport, error) {
	panic("unexpected")
}
func (stubInventoryService) GetSheet(context.Context, uuid.UUID) (BoardSheet, error) {
	panic("unexpected")
}
func (stubInventoryService) ListAvailableSheets(context.Context, httpkit.PageParams, *uuid.UUID) (httpkit.PagedResult[BoardSheet], error) {
	panic("unexpected")
}
func (stubInventoryService) CountAvailableSheetsByMaterial(context.Context, uuid.UUID) (int, error) {
	panic("unexpected")
}
func (stubInventoryService) GetOverflowStatus(context.Context) (OverflowStatus, error) {
	panic("unexpected")
}
func (stubInventoryService) PreAssignSheet(context.Context, PreAssignSheetInput) error {
	panic("unexpected")
}
func (stubInventoryService) RecordCut(context.Context, RecordCutInput) (CutResult, error) {
	panic("unexpected")
}
func (stubInventoryService) ListRemnants(context.Context, RemnantFilter, httpkit.PageParams) (httpkit.PagedResult[Remnant], error) {
	panic("unexpected")
}
func (stubInventoryService) GetRemnant(context.Context, uuid.UUID) (Remnant, error) {
	panic("unexpected")
}
func (stubInventoryService) FindAvailableRemnants(context.Context, domain.Dimension) ([]Remnant, error) {
	panic("unexpected")
}
func (stubInventoryService) SuggestRemnants(context.Context, SuggestRemnantsInput) ([]RemnantSuggestion, error) {
	panic("unexpected")
}
func (stubInventoryService) AllocateRemnant(context.Context, uuid.UUID, uuid.UUID) error {
	panic("unexpected")
}
func (stubInventoryService) MarkRemnantWaste(context.Context, uuid.UUID) error {
	panic("unexpected")
}
func (stubInventoryService) StockRemnant(context.Context, uuid.UUID, string) error {
	panic("unexpected")
}
func (stubInventoryService) GetRemnantLineage(context.Context, uuid.UUID) ([]Remnant, error) {
	panic("unexpected")
}
func (stubInventoryService) GetRemnantLineageByRemnant(context.Context, uuid.UUID) ([]Remnant, error) {
	panic("unexpected")
}
func (stubInventoryService) ReleaseExpiredAllocations(context.Context, time.Time) (int, error) {
	panic("unexpected")
}
func (stubInventoryService) GetRemnantAging(context.Context, int, int) (RemnantAgingSummary, error) {
	panic("unexpected")
}
func (stubInventoryService) ExpireStaleRemnants(context.Context, int) (int, error) {
	panic("unexpected")
}
func (stubInventoryService) ListStorageLocations(context.Context) ([]StorageLocation, error) {
	panic("unexpected")
}
func (stubInventoryService) Transfer(context.Context, TransferInput) (TransferResult, error) {
	panic("unexpected")
}
func (stubInventoryService) ListAuditLog(context.Context, uuid.UUID, string, httpkit.CursorParams) (httpkit.CursorResult[AuditLogEntry], error) {
	panic("unexpected")
}
func (stubInventoryService) ListAuditLogByAction(context.Context, string, httpkit.CursorParams) (httpkit.CursorResult[AuditLogEntry], error) {
	panic("unexpected")
}
func (stubInventoryService) LogRemnantBypass(context.Context, LogRemnantBypassInput) error {
	panic("unexpected")
}
func (stubInventoryService) CreateCycleCountSession(context.Context, CreateCycleCountInput) (CycleCountSession, error) {
	panic("unexpected")
}
func (stubInventoryService) GetCycleCountSession(context.Context, uuid.UUID) (CycleCountSession, error) {
	panic("unexpected")
}
func (stubInventoryService) AddCycleCountLine(context.Context, AddCountLineInput) (CycleCountLine, error) {
	panic("unexpected")
}
func (stubInventoryService) ListCycleCountLines(context.Context, uuid.UUID) ([]CycleCountLine, error) {
	panic("unexpected")
}
func (stubInventoryService) PostCycleCount(context.Context, PostCycleCountInput) error {
	panic("unexpected")
}
func (stubInventoryService) CancelCycleCountSession(context.Context, uuid.UUID, uuid.UUID) error {
	panic("unexpected")
}
func (stubInventoryService) GenerateRemnantLabelPDF(context.Context, RemnantLabelInput) ([]byte, error) {
	panic("unexpected")
}
func (stubInventoryService) GenerateCutLabelsPDF(context.Context, CutLabelsInput) ([]byte, error) {
	panic("unexpected")
}
func (stubInventoryService) GeneratePickSlipPDF(context.Context, uuid.UUID) ([]byte, error) {
	panic("unexpected")
}
func (stubInventoryService) ListCuttingRecords(context.Context, CuttingRecordFilter, httpkit.CursorParams) (httpkit.CursorResult[CuttingRecordReport], error) {
	panic("unexpected")
}

var _ Service = stubInventoryService{}

func newInventoryTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(stubInventoryService{})
	r := gin.New()
	// inject a worker-tier identity so RequireWorkerUp() passes
	r.Use(func(c *gin.Context) {
		c.Set("auth_identity", auth.Identity{
			UserID: uuid.New().String(),
			Role:   auth.RoleWarehouse,
		})
		c.Next()
	})
	h.Register(r.Group("/api/v1"))
	return r
}

func TestListRejections_NoFilter_Returns200(t *testing.T) {
	r := newInventoryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/material-rejections", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListRejections_ValidDateRange_Returns200(t *testing.T) {
	r := newInventoryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/material-rejections?from=2026-05-01&to=2026-06-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListRejections_OnlyFrom_Returns400(t *testing.T) {
	r := newInventoryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/material-rejections?from=2026-05-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when only from provided, got %d", w.Code)
	}
}

func TestListRejections_OnlyTo_Returns400(t *testing.T) {
	r := newInventoryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/material-rejections?to=2026-06-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when only to provided, got %d", w.Code)
	}
}

func TestListRejections_FromAfterTo_Returns400(t *testing.T) {
	r := newInventoryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/material-rejections?from=2026-06-01&to=2026-05-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when from > to, got %d", w.Code)
	}
}

func TestListRejections_InvalidFromFormat_Returns400(t *testing.T) {
	r := newInventoryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/material-rejections?from=not-a-date&to=2026-06-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid from date, got %d", w.Code)
	}
}

func TestListRejections_RFC3339DateRange_Returns200(t *testing.T) {
	r := newInventoryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/material-rejections?from=2026-05-01T00:00:00Z&to=2026-06-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for RFC3339 range, got %d: %s", w.Code, w.Body.String())
	}
}
