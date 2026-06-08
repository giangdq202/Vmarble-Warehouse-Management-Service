package fxrates

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

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.PUT("/fx-rates/:currency/:date", auth.RequireAdminOnly(), h.upsert)
	rg.GET("/fx-rates", h.list)
	rg.DELETE("/fx-rates/:id", auth.RequireAdminOnly(), h.delete)
}

func (h *Handler) upsert(c *gin.Context) {
	currency := c.Param("currency")
	dateStr := c.Param("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD"})
		return
	}
	var body struct {
		RateToVND float64 `json:"rate_to_vnd" binding:"required"`
	}
	if !httpkit.Bind(c, &body) {
		return
	}
	rate, err := h.svc.UpsertRate(c.Request.Context(), UpsertFXRateInput{
		Currency:      currency,
		RateToVND:     body.RateToVND,
		EffectiveDate: date,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, rate)
}

func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	f := FXRateListFilter{Currency: c.Query("currency")}
	res, err := h.svc.ListRates(c.Request.Context(), p, f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeleteRate(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
