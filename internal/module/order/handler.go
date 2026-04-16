package order

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/pos", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.create)
	rg.GET("/pos", h.list)
	rg.GET("/pos/:id", h.get)
	rg.DELETE("/pos/:id", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.delete)
	rg.GET("/pos/:id/line-items", h.lineItems)
}

// createPO godoc
//
// @Summary      Create PO
// @Tags         order
// @Accept       json
// @Produce      json
// @Param        body  body      CreatePOInput  true  "payload"
// @Success      201   {object}  PO
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/pos [post]
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

// listPOs godoc
//
// @Summary      List POs
// @Tags         order
// @Produce      json
// @Param        page     query     int     false  "page number (default 1)"
// @Param        limit    query     int     false  "items per page (default 10, max 100)"
// @Param        search   query     string  false  "search by PO code (ILIKE)"
// @Param        sort_by  query     string  false  "sort column: code, expected_delivery (default created_at)"
// @Param        order    query     string  false  "sort direction: asc, desc (default desc)"
// @Success      200  {object}  httpkit.PagedResult[PO]
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/pos [get]
func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	result, err := h.svc.ListPOs(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// getPO godoc
//
// @Summary      Get PO
// @Tags         order
// @Produce      json
// @Param        id   path      string  true  "po id (uuid)"
// @Success      200  {object}  PO
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/pos/{id} [get]
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
	if !po.IsActive {
		httpkit.Error(c, domain.NewBizError(domain.ErrNotFound, "purchase order not found"))
		return
	}
	c.JSON(http.StatusOK, po)
}

// deletePO godoc
//
// @Summary      Void PO (soft delete)
// @Tags         order
// @Produce      json
// @Param        id   path      string  true  "po id (uuid)"
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/pos/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeactivatePO(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// listPOLineItems godoc
//
// @Summary      List PO line items
// @Tags         order
// @Produce      json
// @Param        id   path      string  true  "po id (uuid)"
// @Success      200  {array}   LineItem
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/pos/{id}/line-items [get]
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
