package scrap

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

var scrapSalesFilterLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	return loc
}()

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/scrap-sales", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.createScrapSale)
	rg.GET("/scrap-sales", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.listScrapSales)
}

// createScrapSale godoc
//
// @Summary      Record a scrap sale transaction (BR-C05/C08)
// @Description  Records a scrap sale. Phase A: only VND currency is accepted.
// @Description  Scrap sales offset waste cost in the WasteReport (BR-C06).
// @Tags         scrap
// @Accept       json
// @Produce      json
// @Param        body  body      CreateScrapSaleInput  true  "payload"
// @Success      201   {object}  ScrapSale
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/scrap-sales [post]
func (h *Handler) createScrapSale(c *gin.Context) {
	var in CreateScrapSaleInput
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return
	}
	in.CreatedBy = createdBy
	sale, err := h.svc.CreateScrapSale(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, sale)
}

// listScrapSales godoc
//
// @Summary      List scrap sales (keyset pagination)
// @Description  Returns scrap sales ordered by created_at DESC. Filter by
// @Description  sale_date range and/or material_id. Period filter matches
// @Description  WasteReport filter (BR-C07).
// @Tags         scrap
// @Produce      json
// @Param        cursor       query     string  false  "opaque keyset cursor from a previous response (omit for first page)"
// @Param        limit        query     int     false  "items per page (default 10, max 100)"
// @Param        from         query     string  false  "from date (Asia/Ho_Chi_Minh, YYYY-MM-DD)"
// @Param        to           query     string  false  "to date inclusive (Asia/Ho_Chi_Minh, YYYY-MM-DD)"
// @Param        material_id  query     string  false  "filter by material id (uuid)"
// @Success      200  {object}  httpkit.CursorResult[ScrapSale]
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/scrap-sales [get]
func (h *Handler) listScrapSales(c *gin.Context) {
	params := httpkit.BindCursorParams(c)
	from, to, ok := parseScrapSalesDateRange(c)
	if !ok {
		return
	}

	var materialID *uuid.UUID
	if v := c.Query("material_id"); v != "" {
		mid, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid material_id"})
			return
		}
		materialID = &mid
	}

	result, err := h.svc.ListScrapSales(c.Request.Context(), params, ListScrapSalesFilter{
		From:       from,
		To:         to,
		MaterialID: materialID,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseScrapSalesDateRange(c *gin.Context) (from, to *time.Time, ok bool) {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" && toStr == "" {
		return nil, nil, true
	}
	if fromStr != "" {
		day, err := time.ParseInLocation(time.DateOnly, fromStr, scrapSalesFilterLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from, use YYYY-MM-DD"})
			return nil, nil, false
		}
		from = &day
	}
	if toStr != "" {
		day, err := time.ParseInLocation(time.DateOnly, toStr, scrapSalesFilterLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to, use YYYY-MM-DD"})
			return nil, nil, false
		}
		// to is inclusive day; query uses half-open [from, to) so add 1 day.
		exclusive := day.AddDate(0, 0, 1)
		to = &exclusive
	}
	return from, to, true
}
