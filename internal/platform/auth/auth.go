package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleAccountant Role = "accountant"
	RolePlanner    Role = "planner"
	RoleWarehouse  Role = "warehouse"
	RoleCNC        Role = "cnc"
	RoleForeman    Role = "foreman"
)

type Identity struct {
	UserID string
	Role   Role
}

const identityKey = "auth_identity"

// FromContext extracts the identity set by the auth middleware.
func FromContext(c *gin.Context) (Identity, bool) {
	v, ok := c.Get(identityKey)
	if !ok {
		return Identity{}, false
	}
	id, ok := v.(Identity)
	return id, ok
}

// Middleware is a placeholder auth middleware.
// Replace with real JWT / session validation.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := Identity{
			UserID: c.GetHeader("X-User-ID"),
			Role:   Role(c.GetHeader("X-User-Role")),
		}
		c.Set(identityKey, id)
		c.Next()
	}
}

// RequireRole returns middleware that rejects requests without the required role.
func RequireRole(roles ...Role) gin.HandlerFunc {
	allowed := make(map[Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		id, ok := FromContext(c)
		if !ok || !allowed[id.Role] {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}
		c.Next()
	}
}
