package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ── Roles ────────────────────────────────────────────────────────────────────

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleAccountant Role = "accountant"
	RolePlanner    Role = "planner"
	RoleWarehouse  Role = "warehouse"
	RoleCNC        Role = "cnc"
	RoleForeman    Role = "foreman"
)

// validRoles contains all recognised roles for fast lookup.
var validRoles = map[Role]bool{
	RoleAdmin:      true,
	RoleAccountant: true,
	RolePlanner:    true,
	RoleWarehouse:  true,
	RoleCNC:        true,
	RoleForeman:    true,
}

// ── Identity ─────────────────────────────────────────────────────────────────

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

// ── Token format ─────────────────────────────────────────────────────────────
//
// Token = base64url(payload) + "." + base64url(HMAC-SHA256(base64url(payload), secret))
// payload is JSON: {"user_id":"...","role":"...","exp":unix_timestamp}
//
// This is NOT a JWT — it is a minimal HMAC-signed token sufficient to prevent
// header spoofing on staging. Replace with real JWT/IdP auth before production.

type tokenPayload struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	Exp    int64  `json:"exp"`
}

// SignToken creates an HMAC-signed token. Exported for tests and internal
// tooling (e.g. generating tokens for curl / Postman).
func SignToken(secret string, userID string, role Role, exp time.Time) string {
	p := tokenPayload{
		UserID: userID,
		Role:   string(role),
		Exp:    exp.Unix(),
	}
	payloadJSON, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := computeHMAC(payloadB64, secret)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return payloadB64 + "." + sigB64
}

func computeHMAC(message, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

// ── Middleware ────────────────────────────────────────────────────────────────

// Middleware returns a gin middleware that verifies HMAC-signed bearer tokens.
// Requests without a valid token receive 401 Unauthorized.
func Middleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		// Split into payload + signature.
		parts := strings.SplitN(token, ".", 2)
		if len(parts) != 2 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "malformed token"})
			c.Abort()
			return
		}
		payloadB64, sigB64 := parts[0], parts[1]

		// Verify HMAC signature (constant-time comparison).
		expectedSig := computeHMAC(payloadB64, secret)
		givenSig, err := base64.RawURLEncoding.DecodeString(sigB64)
		if err != nil || !hmac.Equal(expectedSig, givenSig) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token signature"})
			c.Abort()
			return
		}

		// Decode payload.
		payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "malformed token payload"})
			c.Abort()
			return
		}

		var tp tokenPayload
		if err := json.Unmarshal(payloadJSON, &tp); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "malformed token payload"})
			c.Abort()
			return
		}

		// Check expiration.
		if time.Now().Unix() > tp.Exp {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
			c.Abort()
			return
		}

		// Validate role.
		role := Role(tp.Role)
		if !validRoles[role] {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid role in token"})
			c.Abort()
			return
		}

		c.Set(identityKey, Identity{
			UserID: tp.UserID,
			Role:   role,
		})
		c.Next()
	}
}

// ── Role guard ───────────────────────────────────────────────────────────────

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
