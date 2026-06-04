package shipping

import (
	"net/http"
	"time"

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
	v := rg.Group("/vessels")
	v.POST("", auth.RequirePlannerUp(), h.createVessel)
	v.GET("", auth.RequireWorkerUp(), h.listVessels)
	v.GET("/:id", auth.RequireWorkerUp(), h.getVessel)
	v.PUT("/:id", auth.RequirePlannerUp(), h.updateVessel)
	v.DELETE("/:id", auth.RequireAdminOnly(), h.deleteVessel)

	v.POST("/:id/bookings", auth.RequirePlannerUp(), h.bookContainer)
	v.DELETE("/:id/bookings/:container_id", auth.RequirePlannerUp(), h.unbookContainer)
	v.GET("/:id/bookings", auth.RequireWorkerUp(), h.listContainerBookings)
}

func (h *Handler) createVessel(c *gin.Context) {
	var in CreateVesselInput
	if !httpkit.Bind(c, &in) {
		return
	}
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	createdBy, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	in.CreatedBy = createdBy
	v, err := h.svc.CreateVessel(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (h *Handler) getVessel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vessel id"})
		return
	}
	v, err := h.svc.GetVessel(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) listVessels(c *gin.Context) {
	params := httpkit.BindPageParams(c)
	if err := httpkit.ValidateOffset(params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var f VesselListFilter
	if v := c.Query("cutoff_from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cutoff_from, use RFC3339"})
			return
		}
		f.CutoffFrom = &t
	}

	result, err := h.svc.ListVessels(c.Request.Context(), params, f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) updateVessel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vessel id"})
		return
	}
	var in UpdateVesselInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.ID = id
	v, err := h.svc.UpdateVessel(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) deleteVessel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vessel id"})
		return
	}
	if err := h.svc.DeleteVessel(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) bookContainer(c *gin.Context) {
	vesselID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vessel id"})
		return
	}
	var in BookContainerInput
	if !httpkit.Bind(c, &in) {
		return
	}
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	bookedBy, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	in.VesselID = vesselID
	in.BookedBy = bookedBy
	booking, err := h.svc.BookContainer(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, booking)
}

func (h *Handler) unbookContainer(c *gin.Context) {
	vesselID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vessel id"})
		return
	}
	containerID, err := uuid.Parse(c.Param("container_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container id"})
		return
	}
	if err := h.svc.UnbookContainer(c.Request.Context(), vesselID, containerID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listContainerBookings(c *gin.Context) {
	vesselID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vessel id"})
		return
	}
	bookings, err := h.svc.ListContainerBookings(c.Request.Context(), vesselID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, bookings)
}
