package authn

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// Handler exposes the authn HTTP endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates an authn Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the authn routes on rg.
// rg must NOT have the auth.Middleware applied — login is a public endpoint.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/login", h.login)
}

// login godoc
//
// @Summary      Login
// @Description  Authenticate with username + password and receive a Bearer token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LoginInput   true  "credentials"
// @Success      200   {object}  LoginResult
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/auth/login [post]
func (h *Handler) login(c *gin.Context) {
	var in LoginInput
	if !httpkit.Bind(c, &in) {
		return
	}
	result, err := h.svc.Login(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
