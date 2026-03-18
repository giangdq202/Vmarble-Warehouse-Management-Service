package planning

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/plans", h.create)
	rg.GET("/plans", h.list)
	rg.GET("/plans/:id", h.get)
	rg.POST("/plans/:id/approve", h.approve)
	rg.POST("/plans/:id/cancel", h.cancel)
}

// createPlan godoc
//
// @Summary      Create production plan
// @Tags         planning
// @Accept       json
// @Produce      json
// @Param        body  body      CreatePlanInput  true  "payload"
// @Success      201   {object}  Plan
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/plans [post]
func (h *Handler) create(c *gin.Context) {
	var in CreatePlanInput
	if !httpkit.Bind(c, &in) {
		return
	}
	plan, err := h.svc.CreatePlan(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, plan)
}

// listPlans godoc
//
// @Summary      List production plans
// @Tags         planning
// @Produce      json
// @Success      200  {array}   Plan
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/plans [get]
func (h *Handler) list(c *gin.Context) {
	plans, err := h.svc.ListPlans(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, plans)
}

// getPlan godoc
//
// @Summary      Get production plan
// @Tags         planning
// @Produce      json
// @Param        id   path      string  true  "plan id (uuid)"
// @Success      200  {object}  Plan
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/plans/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	plan, err := h.svc.GetPlan(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, plan)
}

// approvePlan godoc
//
// @Summary      Approve production plan
// @Tags         planning
// @Produce      json
// @Param        id   path      string  true  "plan id (uuid)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Router       /api/v1/plans/{id}/approve [post]
func (h *Handler) approve(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.ApprovePlan(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "approved"})
}

// cancelPlan godoc
//
// @Summary      Cancel production plan
// @Tags         planning
// @Produce      json
// @Param        id   path      string  true  "plan id (uuid)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Router       /api/v1/plans/{id}/cancel [post]
func (h *Handler) cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.CancelPlan(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "canceled"})
}
