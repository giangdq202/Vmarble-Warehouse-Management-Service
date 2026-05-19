package purchasing

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/purchase-orders")
	g.POST("", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.POST("/:id/items", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.addItem)
	g.DELETE("/:id/items/:item_id", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.removeItem)
	g.POST("/:id/order", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.orderPO)
	g.POST("/:id/receive", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.receivePO)
	g.POST("/:id/cancel", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.cancelPO)
}

// create godoc
//
// @Summary      Create a purchase order (DRAFT)
// @Tags         purchasing
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreatePOInput  true  "payload"
// @Success      201   {object}  PurchaseOrder
// @Failure      400   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Router       /api/v1/purchase-orders [post]
func (h *Handler) create(c *gin.Context) {
	var in CreatePOInput
	if !httpkit.Bind(c, &in) {
		return
	}
	if identity, ok := auth.FromContext(c); ok {
		if id, err := uuid.Parse(identity.UserID); err == nil {
			in.CreatedBy = id
		}
	}
	po, err := h.svc.CreatePO(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, po)
}

// list godoc
//
// @Summary      List purchase orders
// @Tags         purchasing
// @Produce      json
// @Security     BearerAuth
// @Param        status      query   string  false  "filter by status (DRAFT|ORDERED|RECEIVED|CANCELLED)"
// @Param        material_id query   string  false  "filter by material UUID"
// @Param        page        query   int     false  "page number"
// @Param        limit       query   int     false  "items per page"
// @Param        from        query   string  false  "filter created_at >= from (YYYY-MM-DD or RFC3339, Asia/Ho_Chi_Minh local)"
// @Param        to          query   string  false  "filter created_at < to + 1 day (YYYY-MM-DD or RFC3339, Asia/Ho_Chi_Minh local; inclusive)"
// @Success      200  {object}  httpkit.PagedResult[PurchaseOrder]
// @Failure      400  {object}  map[string]string
// @Router       /api/v1/purchase-orders [get]
func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	f := POListFilter{
		Status: c.Query("status"),
	}
	if raw := c.Query("material_id"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			f.MaterialID = &id
		}
	}
	from, to, ok := httpkit.ParseDateRangeFilter(c)
	if !ok {
		return
	}
	f.From = from
	f.To = to
	result, err := h.svc.ListPOs(c.Request.Context(), p, f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// get godoc
//
// @Summary      Get a purchase order by ID
// @Tags         purchasing
// @Produce      json
// @Security     BearerAuth
// @Param        id   path   string  true  "PO UUID"
// @Success      200  {object}  PurchaseOrder
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/purchase-orders/{id} [get]
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

// addItem godoc
//
// @Summary      Add an item to a DRAFT purchase order
// @Tags         purchasing
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path   string        true  "PO UUID"
// @Param        body  body   AddPOItemInput true  "item payload"
// @Success      201  {object}  POItem
// @Failure      400  {object}  map[string]string
// @Failure      412  {object}  map[string]string
// @Router       /api/v1/purchase-orders/{id}/items [post]
func (h *Handler) addItem(c *gin.Context) {
	poID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in AddPOItemInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.POID = poID
	item, err := h.svc.AddItem(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

// removeItem godoc
//
// @Summary      Remove an item from a DRAFT purchase order
// @Tags         purchasing
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string  true  "PO UUID"
// @Param        item_id  path  string  true  "Item UUID"
// @Success      204
// @Failure      404  {object}  map[string]string
// @Failure      412  {object}  map[string]string
// @Router       /api/v1/purchase-orders/{id}/items/{item_id} [delete]
func (h *Handler) removeItem(c *gin.Context) {
	poID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	itemID, err := uuid.Parse(c.Param("item_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid item_id"})
		return
	}
	if err := h.svc.RemoveItem(c.Request.Context(), poID, itemID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// orderPO godoc
//
// @Summary      Transition a DRAFT PO to ORDERED
// @Tags         purchasing
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "PO UUID"
// @Success      200  {object}  PurchaseOrder
// @Failure      409  {object}  map[string]string
// @Failure      412  {object}  map[string]string
// @Router       /api/v1/purchase-orders/{id}/order [post]
func (h *Handler) orderPO(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	po, err := h.svc.OrderPO(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, po)
}

// receivePO godoc
//
// @Summary      Mark an ORDERED PO as RECEIVED and create inventory lots
// @Tags         purchasing
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "PO UUID"
// @Success      200  {object}  PurchaseOrder
// @Failure      409  {object}  map[string]string
// @Router       /api/v1/purchase-orders/{id}/receive [post]
func (h *Handler) receivePO(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	po, err := h.svc.ReceivePO(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, po)
}

// cancelPO godoc
//
// @Summary      Cancel a DRAFT or ORDERED purchase order
// @Tags         purchasing
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "PO UUID"
// @Success      200  {object}  PurchaseOrder
// @Failure      409  {object}  map[string]string
// @Router       /api/v1/purchase-orders/{id}/cancel [post]
func (h *Handler) cancelPO(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	po, err := h.svc.CancelPO(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, po)
}
