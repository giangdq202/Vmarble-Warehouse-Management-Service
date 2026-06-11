package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
)

type stubPresigner struct{}

func (stubPresigner) Presign(_ context.Context, contentType string) (PresignResult, error) {
	return PresignResult{
		UploadURL: "https://example.r2.cloudflarestorage.com/uploads/stub.jpg?X-Amz-Signature=xxx",
		PublicURL: "https://pub.r2.dev/uploads/stub.jpg",
	}, nil
}

func newStorageTestRouter(p Presigner) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth_identity", auth.Identity{
			UserID: uuid.New().String(),
			Role:   auth.RoleWarehouse,
		})
		c.Next()
	})
	NewHandler(p).Register(r.Group("/api/v1"))
	return r
}

func TestPresign_ValidJPEG_Returns200(t *testing.T) {
	r := newStorageTestRouter(stubPresigner{})
	body, _ := json.Marshal(map[string]string{"content_type": "image/jpeg"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var res PresignResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if res.UploadURL == "" || res.PublicURL == "" {
		t.Fatalf("expected non-empty URLs, got %+v", res)
	}
}

func TestPresign_NilPresigner_Returns503(t *testing.T) {
	r := newStorageTestRouter(nil)
	body, _ := json.Marshal(map[string]string{"content_type": "image/png"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 when presigner not configured, got %d", w.Code)
	}
}

func TestPresign_MissingContentType_Returns400(t *testing.T) {
	r := newStorageTestRouter(stubPresigner{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing content_type, got %d", w.Code)
	}
}

func TestPresign_InvalidContentType_Returns400(t *testing.T) {
	r := newStorageTestRouter(stubPresigner{})
	body, _ := json.Marshal(map[string]string{"content_type": "application/pdf"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unsupported content_type, got %d", w.Code)
	}
}
