package authn

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
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

// Register mounts the public authn routes on rg.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/login", h.login)
}

// RegisterAdmin mounts the protected administrative routes on rg.
// rg should already have auth.Middleware applied.
func (h *Handler) RegisterAdmin(rg *gin.RouterGroup) {
	users := rg.Group("/users", auth.RequireRole(auth.RoleAdmin))
	{
		users.GET("", h.listUsers)
		users.POST("", h.createUser)
		users.GET("/:id", h.getUserDetail)
		users.PUT("/:id", h.updateUser)
		users.PUT("/:id/password", h.updatePassword)
		users.DELETE("/:id", h.deactivateUser)
	}
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

// listUsers godoc
//
// @Summary      List all users (paginated & filtered)
// @Tags         admin
// @Produce      json
// @Param        page       query     int     false  "page number"
// @Param        limit      query     int     false  "items per page"
// @Param        search     query     string  false  "search by username"
// @Param        role       query     string  false  "filter by role (comma-separated)"
// @Param        is_active  query     bool    false  "filter by active status"
// @Param        sort_by    query     string  false  "sort by: username|role|created_at"
// @Param        order      query     string  false  "sort order: asc|desc"
// @Success      200  {object}  httpkit.PagedResult[UserDetail]
// @Security     BearerAuth
// @Router       /api/v1/admin/users [get]
func (h *Handler) listUsers(c *gin.Context) {
	params := ListUsersParams{
		PageParams: httpkit.BindPageParams(c),
	}

	if role := c.Query("role"); role != "" {
		params.Roles = strings.Split(role, ",")
	}

	if isActive := c.Query("is_active"); isActive != "" {
		val := isActive == "true"
		params.IsActive = &val
	}

	users, err := h.svc.ListUsers(c.Request.Context(), params)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, users)
}

// createUser godoc
//
// @Summary      Create new user
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        body  body      CreateUserInput  true  "payload"
// @Success      201   {object}  UserDetail
// @Security     BearerAuth
// @Router       /api/v1/admin/users [post]
func (h *Handler) createUser(c *gin.Context) {
	id, _ := auth.FromContext(c)
	actorID, _ := uuid.Parse(id.UserID)

	var in CreateUserInput
	if !httpkit.Bind(c, &in) {
		return
	}
	u, err := h.svc.CreateUser(c.Request.Context(), actorID, in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, u)
}

// getUserDetail godoc
//
// @Summary      Get user detail
// @Tags         admin
// @Produce      json
// @Param        id   path      string  true  "user id (uuid)"
// @Success      200  {object}  UserDetail
// @Security     BearerAuth
// @Router       /api/v1/admin/users/{id} [get]
func (h *Handler) getUserDetail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	u, err := h.svc.GetUserDetail(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}

// updateUser godoc
//
// @Summary      Update user info
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        id    path      string           true  "user id (uuid)"
// @Param        body  body      UpdateUserInput  true  "payload"
// @Success      200   {object}  UserDetail
// @Security     BearerAuth
// @Router       /api/v1/admin/users/{id} [put]
func (h *Handler) updateUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in UpdateUserInput
	if !httpkit.Bind(c, &in) {
		return
	}
	u, err := h.svc.UpdateUser(c.Request.Context(), id, in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}

// updatePassword godoc
//
// @Summary      Admin reset user password
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        id    path      string               true  "user id (uuid)"
// @Param        body  body      UpdatePasswordInput  true  "payload"
// @Success      204
// @Security     BearerAuth
// @Router       /api/v1/admin/users/{id}/password [put]
func (h *Handler) updatePassword(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in UpdatePasswordInput
	if !httpkit.Bind(c, &in) {
		return
	}
	if err := h.svc.UpdatePassword(c.Request.Context(), id, in); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// deactivateUser godoc
//
// @Summary      Deactivate user (soft delete)
// @Tags         admin
// @Produce      json
// @Param        id   path      string  true  "user id (uuid)"
// @Success      204
// @Security     BearerAuth
// @Router       /api/v1/admin/users/{id} [delete]
func (h *Handler) deactivateUser(c *gin.Context) {
	id, _ := auth.FromContext(c)
	actorID, _ := uuid.Parse(id.UserID)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeactivateUser(c.Request.Context(), targetID, actorID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
