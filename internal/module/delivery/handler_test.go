package delivery

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type stubDeliveryService struct{}

func (stubDeliveryService) CreateContainer(context.Context, CreateContainerInput) (Container, error) {
	panic("unexpected")
}
func (stubDeliveryService) GetContainer(context.Context, uuid.UUID) (Container, error) {
	panic("unexpected")
}
func (stubDeliveryService) ListContainers(_ context.Context, _ httpkit.PageParams, _ ContainerListFilter) (httpkit.PagedResult[Container], error) {
	return httpkit.PagedResult[Container]{Items: []Container{}, TotalItems: 0, TotalPages: 1, CurrentPage: 1, Limit: 10}, nil
}
func (stubDeliveryService) AddLine(context.Context, AddLineInput) (AddLineResult, error) {
	panic("unexpected")
}
func (stubDeliveryService) DeleteLine(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	panic("unexpected")
}
func (stubDeliveryService) TransferLine(context.Context, TransferLineInput) (TransferLineResult, error) {
	panic("unexpected")
}
func (stubDeliveryService) Seal(context.Context, SealInput) (Container, error) { panic("unexpected") }
func (stubDeliveryService) Reopen(context.Context, ReopenInput) (Container, error) {
	panic("unexpected")
}
func (stubDeliveryService) Ship(context.Context, ShipInput) (Container, error) {
	panic("unexpected")
}
func (stubDeliveryService) Cancel(context.Context, CancelInput) (Container, error) {
	panic("unexpected")
}
func (stubDeliveryService) ListStatusLog(context.Context, uuid.UUID) ([]ContainerStatusLogEntry, error) {
	panic("unexpected")
}
func (stubDeliveryService) UploadLoadingPlan(context.Context, UploadLoadingPlanInput) (LoadingPlanUploadResult, error) {
	panic("unexpected")
}
func (stubDeliveryService) GetActiveLoadingPlan(context.Context, uuid.UUID) (LoadingPlan, error) {
	panic("unexpected")
}
func (stubDeliveryService) GetLoadingPlan(context.Context, uuid.UUID) (LoadingPlan, error) {
	panic("unexpected")
}
func (stubDeliveryService) DiffLoadingPlans(context.Context, uuid.UUID, uuid.UUID) (LoadingPlanDiff, error) {
	panic("unexpected")
}
func (stubDeliveryService) ApproveLoadingPlan(context.Context, ApproveLoadingPlanInput) (LoadingPlan, error) {
	panic("unexpected")
}
func (stubDeliveryService) ListContainerLinesHistory(context.Context, uuid.UUID, *uuid.UUID) ([]ContainerLineHistoryEntry, error) {
	panic("unexpected")
}
func (stubDeliveryService) ListAtRisk(context.Context, int) ([]AtRiskRow, error) {
	panic("unexpected")
}
func (stubDeliveryService) AssignLoader(context.Context, AssignLoaderInput) (Container, error) {
	panic("unexpected")
}
func (stubDeliveryService) ListLoaderLog(context.Context, uuid.UUID) ([]ContainerLoaderLog, error) {
	panic("unexpected")
}
func (stubDeliveryService) ExportPackingList(context.Context, uuid.UUID, io.Writer) error {
	panic("unexpected")
}
func (stubDeliveryService) ChangeDestination(context.Context, ChangeDestinationInput) (Container, error) {
	panic("unexpected")
}
func (stubDeliveryService) ListRouteLog(context.Context, uuid.UUID) ([]ContainerRouteChangeLog, error) {
	panic("unexpected")
}
func (stubDeliveryService) SetFGComponentChecker(FGComponentChecker) {}

var _ Service = stubDeliveryService{}

func newDeliveryTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(stubDeliveryService{})
	r := gin.New()
	h.Register(r.Group("/api/v1"))
	return r
}

func TestListContainers_NoFilter_Returns200(t *testing.T) {
	r := newDeliveryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListContainers_ValidVesselID_Returns200(t *testing.T) {
	r := newDeliveryTestRouter()
	vid := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers?vessel_id="+vid, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for valid vessel_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListContainers_InvalidVesselID_Returns400(t *testing.T) {
	r := newDeliveryTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers?vessel_id=not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid vessel_id, got %d: %s", w.Code, w.Body.String())
	}
}
