package catalog

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
	rg.POST("/materials", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.createMaterial)
	rg.GET("/materials", h.listMaterials)
	rg.GET("/materials/:id", h.getMaterial)
	rg.DELETE("/materials/:id", auth.RequireRole(auth.RoleAdmin), h.deleteMaterial)

	rg.POST("/skus", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.createSKU)
	rg.GET("/skus", h.listSKUs)
	rg.GET("/skus/:id", h.getSKU)
	rg.DELETE("/skus/:id", auth.RequireRole(auth.RoleAdmin), h.deleteSKU)

	rg.PUT("/skus/:id/bom", auth.RequireRole(auth.RoleWarehouse, auth.RolePlanner, auth.RoleAdmin), h.setBOM)
	rg.GET("/skus/:id/bom", h.getBOM)
}

// createMaterial godoc
//
// @Summary      Create material
// @Tags         catalog
// @Accept       json
// @Produce      json
// @Param        body  body      CreateMaterialInput  true  "payload"
// @Success      201   {object}  Material
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/materials [post]
func (h *Handler) createMaterial(c *gin.Context) {
	var in CreateMaterialInput
	if !httpkit.Bind(c, &in) {
		return
	}
	m, err := h.svc.CreateMaterial(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

// listMaterials godoc
//
// @Summary      List materials (paginated)
// @Tags         catalog
// @Produce      json
// @Param        page     query     int     false  "page number (default 1)"
// @Param        limit    query     int     false  "items per page (default 10, max 100)"
// @Param        search   query     string  false  "filter by name (case-insensitive)"
// @Param        sort_by  query     string  false  "sort column: name|type|unit (default created_at)"
// @Param        order    query     string  false  "asc or desc (default asc)"
// @Success      200  {object}  httpkit.PagedResult[Material]
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/materials [get]
func (h *Handler) listMaterials(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	result, err := h.svc.ListMaterials(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// getMaterial godoc
//
// @Summary      Get material
// @Tags         catalog
// @Produce      json
// @Param        id   path      string  true  "material id (uuid)"
// @Success      200  {object}  Material
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/materials/{id} [get]
func (h *Handler) getMaterial(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	m, err := h.svc.GetMaterial(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	if !m.IsActive {
		httpkit.Error(c, domain.NewBizError(domain.ErrNotFound, "material not found"))
		return
	}
	c.JSON(http.StatusOK, m)
}

// deleteMaterial godoc
//
// @Summary      Deactivate material (soft delete)
// @Tags         catalog
// @Produce      json
// @Param        id   path      string  true  "material id (uuid)"
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/materials/{id} [delete]
func (h *Handler) deleteMaterial(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeactivateMaterial(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// createSKU godoc
//
// @Summary      Create SKU
// @Tags         catalog
// @Accept       json
// @Produce      json
// @Param        body  body      CreateSKUInput  true  "payload"
// @Success      201   {object}  SKU
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/skus [post]
func (h *Handler) createSKU(c *gin.Context) {
	var in CreateSKUInput
	if !httpkit.Bind(c, &in) {
		return
	}
	s, err := h.svc.CreateSKU(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, s)
}

// listSKUs godoc
//
// @Summary      List SKUs (paginated)
// @Tags         catalog
// @Produce      json
// @Param        page     query     int     false  "page number (default 1)"
// @Param        limit    query     int     false  "items per page (default 10, max 100)"
// @Param        search   query     string  false  "filter by name or code (case-insensitive)"
// @Param        sort_by  query     string  false  "sort column: name|code (default created_at)"
// @Param        order    query     string  false  "asc or desc (default asc)"
// @Success      200  {object}  httpkit.PagedResult[SKU]
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/skus [get]
func (h *Handler) listSKUs(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	result, err := h.svc.ListSKUs(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// getSKU godoc
//
// @Summary      Get SKU
// @Tags         catalog
// @Produce      json
// @Param        id   path      string  true  "sku id (uuid)"
// @Success      200  {object}  SKU
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/skus/{id} [get]
func (h *Handler) getSKU(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	s, err := h.svc.GetSKU(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	if !s.IsActive {
		httpkit.Error(c, domain.NewBizError(domain.ErrNotFound, "SKU not found"))
		return
	}
	c.JSON(http.StatusOK, s)
}

// deleteSKU godoc
//
// @Summary      Deactivate SKU (soft delete)
// @Tags         catalog
// @Produce      json
// @Param        id   path      string  true  "sku id (uuid)"
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/skus/{id} [delete]
func (h *Handler) deleteSKU(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeactivateSKU(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// setBOM godoc
//
// @Summary      Set BOM for SKU
// @Tags         catalog
// @Accept       json
// @Produce      json
// @Param        id    path      string        true  "sku id (uuid)"
// @Param        body  body      SetBOMInput   true  "payload"
// @Success      200   {object}  BOM
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/skus/{id}/bom [put]
func (h *Handler) setBOM(c *gin.Context) {
	skuID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in SetBOMInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.SKUID = skuID
	bom, err := h.svc.SetBOM(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, bom)
}

// getBOM godoc
//
// @Summary      Get BOM for SKU
// @Tags         catalog
// @Produce      json
// @Param        id   path      string  true  "sku id (uuid)"
// @Success      200  {object}  BOM
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/skus/{id}/bom [get]
func (h *Handler) getBOM(c *gin.Context) {
	skuID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	bom, err := h.svc.GetBOM(c.Request.Context(), skuID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, bom)
}
