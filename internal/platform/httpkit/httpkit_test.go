package httpkit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type mockPinger struct {
	err error
}

func (m mockPinger) Ping(_ context.Context) error {
	return m.err
}

func TestHealthz_DBConnected_Returns200WithCallerInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter(mockPinger{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("User-Agent", "unit-test-agent")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["db"] != "connected" {
		t.Errorf("db = %v, want connected", body["db"])
	}
	if body["method"] != http.MethodGet {
		t.Errorf("method = %v, want GET", body["method"])
	}
	if body["path"] != "/healthz" {
		t.Errorf("path = %v, want /healthz", body["path"])
	}
	if body["user_agent"] != "unit-test-agent" {
		t.Errorf("user_agent = %v, want unit-test-agent", body["user_agent"])
	}
	if body["client_ip"] == "" {
		t.Error("client_ip should not be empty")
	}
}

func TestHealthz_DBDisconnected_Returns503WithCallerInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter(mockPinger{err: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("User-Agent", "unit-test-agent")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["status"] != "error" {
		t.Errorf("status = %v, want error", body["status"])
	}
	if body["db"] != "disconnected" {
		t.Errorf("db = %v, want disconnected", body["db"])
	}
	if body["method"] != http.MethodGet {
		t.Errorf("method = %v, want GET", body["method"])
	}
	if body["path"] != "/healthz" {
		t.Errorf("path = %v, want /healthz", body["path"])
	}
	if body["user_agent"] != "unit-test-agent" {
		t.Errorf("user_agent = %v, want unit-test-agent", body["user_agent"])
	}
	if body["client_ip"] == "" {
		t.Error("client_ip should not be empty")
	}
}
