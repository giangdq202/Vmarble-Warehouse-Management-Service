package production

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/work-orders", h.create)
	rg.GET("/work-orders", h.list)
	rg.GET("/work-orders/:id", h.get)
	rg.POST("/work-orders/:id/advance", h.advance)
	rg.POST("/work-orders/:id/consumptions", h.recordConsumption)
	rg.GET("/work-orders/:id/consumptions", h.listConsumptions)
}

// createWorkOrder godoc
//
// @Summary      Create work order
// @Tags         production
// @Accept       json
// @Produce      json
// @Param        body  body      CreateWOInput  true  "payload"
// @Success      201   {object}  WorkOrder
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/work-orders [post]
func (h *Handler) create(c *gin.Context) {
	var in CreateWOInput
	if !httpkit.Bind(c, &in) {
		return
	}
	wo, err := h.svc.CreateWorkOrder(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, wo)
}

// listWorkOrders godoc
//
// @Summary      List work orders
// @Tags         production
// @Produce      json
// @Param        plan_id  query     string  false  "filter by plan id (uuid)"
// @Success      200      {array}   WorkOrder
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/work-orders [get]
func (h *Handler) list(c *gin.Context) {
	planIDStr := c.Query("plan_id")
	if planIDStr != "" {
		planID, err := uuid.Parse(planIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan_id"})
			return
		}
		wos, err := h.svc.ListWorkOrdersByPlan(c.Request.Context(), planID)
		if err != nil {
			httpkit.Error(c, err)
			return
		}
		c.JSON(http.StatusOK, wos)
		return
	}
	wos, err := h.svc.ListWorkOrders(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wos)
}

// getWorkOrder godoc
//
// @Summary      Get work order
// @Tags         production
// @Produce      json
// @Param        id   path      string  true  "work order id (uuid)"
// @Success      200  {object}  WorkOrder
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/work-orders/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	wo, err := h.svc.GetWorkOrder(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wo)
}

// advanceWorkOrder godoc
//
// @Summary      Advance work order status
// @Tags         production
// @Accept       json
// @Produce      json
// @Param        id    path      string  true  "work order id (uuid)"
// @Param        body  body      object  true  "payload"  SchemaExample({"status":"IN_CUTTING"})
// @Success      200   {object}  map[string]string
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/advance [post]
func (h *Handler) advance(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Status domain.WorkOrderStatus `json:"status"`
	}
	if !httpkit.Bind(c, &body) {
		return
	}
	if err := h.svc.AdvanceStatus(c.Request.Context(), id, body.Status); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": string(body.Status)})
}

// recordConsumption godoc
//
// @Summary      Record material consumption for work order
// @Tags         production
// @Accept       json
// @Produce      json
// @Param        id    path      string                true  "work order id (uuid)"
// @Param        body  body      RecordConsumptionInput true  "payload"
// @Success      201   {object}  ConsumptionRecord
// @Failure      400   {object}  map[string]string
// @Failure      422   {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/consumptions [post]
func (h *Handler) recordConsumption(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in RecordConsumptionInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.WorkOrderID = id
	cr, err := h.svc.RecordConsumption(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, cr)
}

// listConsumptions godoc
//
// @Summary      List consumption records for work order
// @Tags         production
// @Produce      json
// @Param        id   path      string  true  "work order id (uuid)"
// @Success      200  {array}   ConsumptionRecord
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/consumptions [get]
func (h *Handler) listConsumptions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	records, err := h.svc.ListConsumptions(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, records)
}
