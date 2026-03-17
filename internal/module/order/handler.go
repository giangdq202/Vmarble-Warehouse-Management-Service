package order

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
	rg.POST("/pos", h.create)
	rg.GET("/pos", h.list)
	rg.GET("/pos/:id", h.get)
	rg.GET("/pos/:id/line-items", h.lineItems)
}

func (h *Handler) create(c *gin.Context) {
	var in CreatePOInput
	if !httpkit.Bind(c, &in) {
		return
	}
	po, err := h.svc.CreatePO(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, po)
}

func (h *Handler) list(c *gin.Context) {
	pos, err := h.svc.ListPOs(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, pos)
}

func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	po, err := h.svc.GetPO(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, po)
}

func (h *Handler) lineItems(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	items, err := h.svc.GetLineItemsByPO(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
