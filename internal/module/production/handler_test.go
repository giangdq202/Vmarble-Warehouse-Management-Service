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

type stubService struct{}

func (stubService) CreateWorkOrder(context.Context, CreateWOInput) (WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) GetWorkOrder(context.Context, uuid.UUID) (WorkOrder, error) {
	panic("unexpected call")
}
func (stubService) ListWorkOrders(context.Context, httpkit.PageParams, string, *uuid.UUID) (httpkit.PagedResult[WorkOrder], error) {
	panic("unexpected call")
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

var _ Service = stubService{}

func performAdvanceWithRole(t *testing.T, role auth.Role) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	h := NewHandler(stubService{})
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
