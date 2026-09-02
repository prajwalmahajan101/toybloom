package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prajwalmahajan101/toybloom/internal/core/config"
	"github.com/prajwalmahajan101/toybloom/internal/core/logger"
	"github.com/prajwalmahajan101/toybloom/internal/service"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
)

func setupRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ms := store.NewMemStore()
	svc := service.New(ms)
	h := NewHandler(svc)
	hh := NewHealthHandler(ms)
	cfg := config.Load()
	lg := logger.New("error") // quiet logs during tests
	return NewRouter(h, hh, cfg, lg)
}

func createFilter(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/filters", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func parseData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data          map[string]any `json:"data"`
		CorrelationID string         `json:"correlation_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.CorrelationID == "" {
		t.Error("missing correlation_id in response body")
	}
	return envelope.Data
}

func TestCreateFilter(t *testing.T) {
	r := setupRouter(t)
	w := createFilter(t, r, `{"name":"test","n":1000,"p":0.01}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
	}

	data := parseData(t, w)
	if data["name"] != "test" {
		t.Errorf("name: want test, got %v", data["name"])
	}
	if data["stages"] == nil {
		t.Error("missing stages field")
	}
	if data["m"] == nil {
		t.Error("missing m field")
	}
	if data["k"] == nil {
		t.Error("missing k field")
	}
}

func TestCreateFilter_Duplicate(t *testing.T) {
	r := setupRouter(t)
	createFilter(t, r, `{"name":"dup","n":1000,"p":0.01}`)

	w := createFilter(t, r, `{"name":"dup","n":1000,"p":0.01}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateFilter_InvalidName(t *testing.T) {
	r := setupRouter(t)
	w := createFilter(t, r, `{"name":"bad name!","n":1000,"p":0.01}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
	}

	// The bad-name pattern is enforced only by the spec validator (the old
	// validName handler guard is gone; the service rejects only empty names).
	// Assert the failure comes back in our error envelope.
	var env struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Success {
		t.Error("want success=false")
	}
	if len(env.Errors) == 0 || env.Errors[0].Code != "INVALID_ARGUMENT" {
		t.Errorf("want errors[0].code=INVALID_ARGUMENT, got %+v", env.Errors)
	}
}

func TestCreateFilter_InvalidParams(t *testing.T) {
	r := setupRouter(t)

	cases := []struct {
		name string
		body string
	}{
		{"n=0", `{"name":"f","n":0,"p":0.01}`},
		{"p=2", `{"name":"f","n":1000,"p":2}`},
		{"p=0", `{"name":"f","n":1000,"p":0}`},
		{"missing name", `{"n":1000,"p":0.01}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := createFilter(t, r, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAddItem(t *testing.T) {
	r := setupRouter(t)
	createFilter(t, r, `{"name":"f1","n":1000,"p":0.01}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/filters/f1/items", strings.NewReader(`{"value":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	if data["added"] != true {
		t.Errorf("added: want true, got %v", data["added"])
	}
}

func TestCheckItem_Exists(t *testing.T) {
	r := setupRouter(t)
	createFilter(t, r, `{"name":"f2","n":1000,"p":0.01}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/filters/f2/items", strings.NewReader(`{"value":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/filters/f2/items/hello", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	if data["exists"] != true {
		t.Errorf("exists: want true, got %v", data["exists"])
	}
}

func TestCheckItem_NotExists(t *testing.T) {
	r := setupRouter(t)
	createFilter(t, r, `{"name":"f3","n":1000,"p":0.01}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/filters/f3/items/nope", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	if data["exists"] != false {
		t.Errorf("exists: want false, got %v", data["exists"])
	}
}

func TestCheckItem_NoFilter(t *testing.T) {
	r := setupRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/filters/ghost/items/x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetFilter(t *testing.T) {
	r := setupRouter(t)
	createFilter(t, r, `{"name":"f4","n":1000,"p":0.01}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/filters/f4", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	data := parseData(t, w)
	if data["name"] != "f4" {
		t.Errorf("name: want f4, got %v", data["name"])
	}
}

func TestDeleteFilter(t *testing.T) {
	r := setupRouter(t)
	createFilter(t, r, `{"name":"f5","n":1000,"p":0.01}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v1/filters/f5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/filters/f5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 after delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHealthz(t *testing.T) {
	r := setupRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	r := setupRouter(t) // MemStore.Ping returns nil → ready

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCorrelationID(t *testing.T) {
	r := setupRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/filters/nope", nil)
	r.ServeHTTP(w, req)

	corrID := w.Header().Get("X-Correlation-ID")
	if corrID == "" {
		t.Error("missing X-Correlation-ID header")
	}
}

func TestCorrelationID_Passthrough(t *testing.T) {
	r := setupRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/filters/nope", nil)
	req.Header.Set("X-Correlation-ID", "my-custom-id")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Correlation-ID"); got != "my-custom-id" {
		t.Errorf("want my-custom-id, got %q", got)
	}
}

func TestCorrelationID_InSuccessBody(t *testing.T) {
	r := setupRouter(t)
	createFilter(t, r, `{"name":"corr","n":1000,"p":0.01}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/filters/corr", nil)
	req.Header.Set("X-Correlation-ID", "trace-123")
	r.ServeHTTP(w, req)

	var envelope struct {
		CorrelationID string `json:"correlation_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.CorrelationID != "trace-123" {
		t.Errorf("body correlation_id: want trace-123, got %q", envelope.CorrelationID)
	}
}
