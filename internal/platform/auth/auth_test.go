package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testSecret = "test-secret-32-chars-minimum!!"

// ── helpers ──────────────────────────────────────────────────────────────────

// runMiddleware sets up a gin engine with the auth middleware and an echo
// handler that records the extracted Identity. Returns the response recorder
// and the identity (if any).
func runMiddleware(t *testing.T, secret string, authHeader string) (*httptest.ResponseRecorder, *Identity) {
	t.Helper()

	var captured *Identity

	r := gin.New()
	r.Use(Middleware(secret))
	r.GET("/test", func(c *gin.Context) {
		id, ok := FromContext(c)
		if ok {
			captured = &id
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w, captured
}

// ── Middleware tests ─────────────────────────────────────────────────────────

func TestMiddleware_ValidToken_SetsIdentity(t *testing.T) {
	token := SignToken(testSecret, "user-42", RoleAdmin, time.Now().Add(time.Hour))

	w, id := runMiddleware(t, testSecret, "Bearer "+token)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if id == nil {
		t.Fatal("Identity must be set in context")
	}
	if id.UserID != "user-42" {
		t.Errorf("UserID = %q, want %q", id.UserID, "user-42")
	}
	if id.Role != RoleAdmin {
		t.Errorf("Role = %q, want %q", id.Role, RoleAdmin)
	}
}

func TestMiddleware_MissingToken_Returns401(t *testing.T) {
	w, id := runMiddleware(t, testSecret, "")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if id != nil {
		t.Error("Identity must NOT be set when token is missing")
	}
}

func TestMiddleware_ExpiredToken_Returns401(t *testing.T) {
	token := SignToken(testSecret, "user-99", RoleForeman, time.Now().Add(-time.Hour))

	w, id := runMiddleware(t, testSecret, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if id != nil {
		t.Error("Identity must NOT be set for expired token")
	}
}

func TestMiddleware_InvalidSignature_Returns401(t *testing.T) {
	token := SignToken("wrong-secret", "user-42", RoleAdmin, time.Now().Add(time.Hour))

	w, id := runMiddleware(t, testSecret, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if id != nil {
		t.Error("Identity must NOT be set for invalid signature")
	}
}

// ── Edge cases ───────────────────────────────────────────────────────────────

func TestMiddleware_NoBearerPrefix_Returns401(t *testing.T) {
	token := SignToken(testSecret, "user-1", RoleAdmin, time.Now().Add(time.Hour))

	w, _ := runMiddleware(t, testSecret, "Token "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for non-Bearer prefix", w.Code)
	}
}

func TestMiddleware_MalformedToken_NoDot_Returns401(t *testing.T) {
	w, _ := runMiddleware(t, testSecret, "Bearer nodothere")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for malformed token", w.Code)
	}
}

func TestMiddleware_InvalidRole_Returns401(t *testing.T) {
	// Manually craft a token with a role that doesn't exist in validRoles.
	p := tokenPayload{
		UserID: "user-1",
		Role:   "superadmin",
		Exp:    time.Now().Add(time.Hour).Unix(),
	}
	payloadJSON, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := computeHMAC(payloadB64, testSecret)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	token := payloadB64 + "." + sigB64

	w, _ := runMiddleware(t, testSecret, "Bearer "+token)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for invalid role", w.Code)
	}
}

// ── All valid roles ──────────────────────────────────────────────────────────

func TestMiddleware_AllValidRoles_Accepted(t *testing.T) {
	roles := []Role{
		RoleAdmin, RoleAccountant, RolePlanner,
		RoleWarehouse, RoleCNC, RoleForeman,
	}
	for _, role := range roles {
		role := role
		t.Run(string(role), func(t *testing.T) {
			token := SignToken(testSecret, "user-"+string(role), role, time.Now().Add(time.Hour))
			w, id := runMiddleware(t, testSecret, "Bearer "+token)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 for role %s", w.Code, role)
			}
			if id == nil || id.Role != role {
				t.Errorf("Role = %v, want %s", id, role)
			}
		})
	}
}

// ── RequireRole tests ────────────────────────────────────────────────────────

func runWithRole(t *testing.T, identityRole Role, requiredRoles ...Role) *httptest.ResponseRecorder {
	t.Helper()

	r := gin.New()
	// Manually inject Identity (simulates Middleware having run).
	r.Use(func(c *gin.Context) {
		c.Set(identityKey, Identity{UserID: "u1", Role: identityRole})
		c.Next()
	})
	r.Use(RequireRole(requiredRoles...))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireRole_AllowedRole_PassesThrough(t *testing.T) {
	w := runWithRole(t, RoleAdmin, RoleAdmin, RolePlanner)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for allowed role", w.Code)
	}
}

func TestRequireRole_ForbiddenRole_Returns403(t *testing.T) {
	w := runWithRole(t, RoleCNC, RoleAdmin, RoleAccountant)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for forbidden role", w.Code)
	}
}

func TestRequireRole_NoIdentity_Returns403(t *testing.T) {
	r := gin.New()
	// No Identity middleware — context has no identity.
	r.Use(RequireRole(RoleAdmin))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 when no identity in context", w.Code)
	}
}

// ── SignToken tests ──────────────────────────────────────────────────────────

func TestSignToken_Deterministic(t *testing.T) {
	exp := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	token1 := SignToken(testSecret, "user-1", RoleAdmin, exp)
	token2 := SignToken(testSecret, "user-1", RoleAdmin, exp)

	if token1 != token2 {
		t.Errorf("SignToken is not deterministic:\n  %s\n  %s", token1, token2)
	}
}

func TestSignToken_DifferentSecretsDifferentTokens(t *testing.T) {
	exp := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	token1 := SignToken("secret-a", "user-1", RoleAdmin, exp)
	token2 := SignToken("secret-b", "user-1", RoleAdmin, exp)

	if token1 == token2 {
		t.Error("tokens signed with different secrets must differ")
	}
}
