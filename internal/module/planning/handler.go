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

func (h *Handler) list(c *gin.Context) {
	plans, err := h.svc.ListPlans(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, plans)
}

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
