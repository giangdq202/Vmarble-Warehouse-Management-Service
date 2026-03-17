package catalog

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
	rg.POST("/materials", h.createMaterial)
	rg.GET("/materials", h.listMaterials)
	rg.GET("/materials/:id", h.getMaterial)

	rg.POST("/skus", h.createSKU)
	rg.GET("/skus", h.listSKUs)
	rg.GET("/skus/:id", h.getSKU)

	rg.PUT("/skus/:id/bom", h.setBOM)
	rg.GET("/skus/:id/bom", h.getBOM)
}

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

func (h *Handler) listMaterials(c *gin.Context) {
	materials, err := h.svc.ListMaterials(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, materials)
}

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
	c.JSON(http.StatusOK, m)
}

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

func (h *Handler) listSKUs(c *gin.Context) {
	skus, err := h.svc.ListSKUs(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, skus)
}

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
	c.JSON(http.StatusOK, s)
}

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
