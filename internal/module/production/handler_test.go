package production

import (
	"bytes"
	"context"
	"encoding/json"
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

type stubService struct {
	listResult httpkit.PagedResult[WorkOrder]
}

func (stubService) CreateWorkOrder(context.Context, CreateWOInput) (WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) GetWorkOrder(context.Context, uuid.UUID) (WorkOrder, error) {
	panic("unexpected call")
}
func (s stubService) ListWorkOrders(context.Context, httpkit.PageParams, WorkOrderListFilter) (httpkit.PagedResult[WorkOrder], error) {
	return s.listResult, nil
}
func (stubService) ListWorkOrdersByPlan(context.Context, uuid.UUID) ([]WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) ListWorkOrdersByAssignee(context.Context, uuid.UUID) ([]WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) AdvanceStatus(context.Context, uuid.UUID, AdvanceStatusInput) error { return nil }
func (stubService) RecordConsumption(context.Context, RecordConsumptionInput) (ConsumptionRecord, error) {
	panic("unexpected call")
}
func (stubService) ListConsumptions(context.Context, uuid.UUID) ([]ConsumptionRecord, error) {
	panic("unexpected call")
}
func (stubService) AssignWorkOrder(context.Context, AssignWorkOrderInput) (WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) SuggestAssignment(context.Context, uuid.UUID) (SuggestAssignmentResult, error) {
	panic("unexpected call")
}
func (stubService) CreateMachine(context.Context, CreateMachineInput) (Machine, error) {
	panic("unexpected call")
}
func (stubService) ListMachines(context.Context) ([]Machine, error)        { panic("unexpected call") }
func (stubService) GetMachine(context.Context, uuid.UUID) (Machine, error) { panic("unexpected call") }
func (stubService) DeactivateMachine(context.Context, uuid.UUID) error     { panic("unexpected call") }
func (stubService) CreateSlot(context.Context, CreateSlotInput) (MachineShiftSlot, error) {
	panic("unexpected call")
}
func (stubService) ListSlots(context.Context, uuid.UUID, time.Time, time.Time) ([]MachineShiftSlot, error) {
	panic("unexpected call")
}
func (stubService) GetSlot(context.Context, uuid.UUID) (MachineShiftSlot, error) {
	panic("unexpected call")
}
func (stubService) DeleteSlot(context.Context, uuid.UUID) error { panic("unexpected call") }
func (stubService) SetEstimatedHours(context.Context, SetEstimatedHoursInput) (WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) AssignSlot(context.Context, AssignSlotInput) (WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) UnassignSlot(context.Context, uuid.UUID) (WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) SuggestSchedule(context.Context, uuid.UUID) ([]ScheduleSuggestion, error) {
	panic("unexpected call")
}
func (stubService) RecordLaborEntry(context.Context, RecordLaborEntryInput) (LaborEntry, error) {
	panic("unexpected call")
}
func (stubService) ListLaborEntries(context.Context, uuid.UUID) ([]LaborEntry, error) {
	panic("unexpected call")
}
func (stubService) SumLaborCost(context.Context, uuid.UUID) (domain.Money, error) {
	panic("unexpected call")
}
func (stubService) ListStatusesByPlan(context.Context, uuid.UUID) ([]domain.WorkOrderStatus, error) {
	panic("unexpected call")
}
func (stubService) CancelPlannedByPlan(context.Context, uuid.UUID) (int64, error) {
	panic("unexpected call")
}

var _ Service = stubService{}

func performAdvanceWithRole(t *testing.T, role auth.Role) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	h := NewHandler(stubService{listResult: httpkit.PagedResult[WorkOrder]{Items: []WorkOrder{}, TotalItems: 0, TotalPages: 1, CurrentPage: 1, Limit: 10}})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth_identity", auth.Identity{UserID: uuid.NewString(), Role: role})
		c.Next()
	})
	h.Register(r.Group("/api/v1"))

	body, _ := json.Marshal(AdvanceStatusInput{To: domain.WOInCutting})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders/"+uuid.NewString()+"/advance", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdvanceRoute_AdminRole_IsAllowed(t *testing.T) {
	w := performAdvanceWithRole(t, auth.RoleAdmin)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for admin", w.Code)
	}
}

func TestAdvanceRoute_CNCRole_IsAllowed(t *testing.T) {
	w := performAdvanceWithRole(t, auth.RoleCNC)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for cnc", w.Code)
	}
}

func TestAdvanceRoute_AccountantRole_IsForbidden(t *testing.T) {
	w := performAdvanceWithRole(t, auth.RoleAccountant)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for accountant", w.Code)
	}
}

func TestListRoute_DateFilter_InvalidDate_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(stubService{listResult: httpkit.PagedResult[WorkOrder]{Items: []WorkOrder{}, TotalItems: 0, TotalPages: 1, CurrentPage: 1, Limit: 10}})
	r := gin.New()
	h.Register(r.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?date=26-04-2026", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestListRoute_DateFilter_RequiresBothFromAndTo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(stubService{listResult: httpkit.PagedResult[WorkOrder]{Items: []WorkOrder{}, TotalItems: 0, TotalPages: 1, CurrentPage: 1, Limit: 10}})
	r := gin.New()
	h.Register(r.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?from=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestListRoute_DateFilter_SupportsLocalTodayDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(stubService{listResult: httpkit.PagedResult[WorkOrder]{Items: []WorkOrder{}, TotalItems: 0, TotalPages: 1, CurrentPage: 1, Limit: 10}})
	r := gin.New()
	h.Register(r.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?date=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func newListHandler() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(stubService{listResult: httpkit.PagedResult[WorkOrder]{Items: []WorkOrder{}, TotalItems: 0, TotalPages: 1, CurrentPage: 1, Limit: 10}})
	r := gin.New()
	h.Register(r.Group("/api/v1"))
	return r
}

func TestListRoute_DashboardPreset_Returns200(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?preset=dashboard_default", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for dashboard_default preset", w.Code)
	}
}

func TestListRoute_DashboardPreset_UnknownPreset_Returns400(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?preset=invalid_preset", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for unknown preset", w.Code)
	}
}

func TestListRoute_DashboardPreset_CannotCombineWithStatus_Returns400(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?preset=dashboard_default&status=PLANNED", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when preset combined with status", w.Code)
	}
}

func TestListRoute_DashboardPreset_CannotCombineWithDate_Returns400(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?preset=dashboard_default&date=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when preset combined with date", w.Code)
	}
}

func TestListRoute_DashboardPreset_CannotCombineWithFromTo_Returns400(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?preset=dashboard_default&from=2026-04-25&to=2026-04-26", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when preset combined with from/to", w.Code)
	}
}

func TestListRoute_AssignedNull_Returns200(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?assigned=null", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for assigned=null", w.Code)
	}
}

func TestListRoute_AssignedToUUID_Returns200(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?assigned="+uuid.NewString(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for assigned=<uuid>", w.Code)
	}
}

func TestListRoute_AssignedInvalidUUID_Returns400(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?assigned=not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for malformed assigned uuid", w.Code)
	}
}

func TestListRoute_DashboardPreset_CannotCombineWithAssigned_Returns400(t *testing.T) {
	r := newListHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/work-orders?preset=dashboard_default&assigned=null", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when preset combined with assigned", w.Code)
	}
}
