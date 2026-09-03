//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// envelope mirrors internal/core/response.Envelope for decoding responses.
type envelope struct {
	Success       bool            `json:"success"`
	Message       string          `json:"message"`
	Data          json.RawMessage `json:"data"`
	CorrelationID string          `json:"correlation_id"`
}

// TestHTTPEndToEnd walks the real API against a running server. It is skipped
// unless E2E_BASE_URL is set, so it stays inert during `go test -tags=e2e ./...`
// runs that have no server. `make e2e` sets the variable and boots compose.
func TestHTTPEndToEnd(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set; skipping HTTP e2e (run via `make e2e`)")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	name := fmt.Sprintf("e2e-%d", time.Now().UnixNano())

	// 1) Create the filter.
	env := do(t, client, http.MethodPost, base+"/v1/filters",
		map[string]any{"name": name, "n": 1000, "p": 0.01}, http.StatusCreated)
	if !env.Success {
		t.Fatalf("create: success=false: %s", env.Message)
	}

	// 2) Add an item.
	do(t, client, http.MethodPost, base+"/v1/filters/"+name+"/items",
		map[string]any{"value": "a@b.com"}, http.StatusOK)

	// 3) Existing item → exists:true (zero false negative on the wire).
	env = do(t, client, http.MethodGet, base+"/v1/filters/"+name+"/items/a@b.com", nil, http.StatusOK)
	assertExists(t, env, true)

	// 4) Never-added item → exists:false (definitely absent).
	env = do(t, client, http.MethodGet, base+"/v1/filters/"+name+"/items/never-added-value", nil, http.StatusOK)
	assertExists(t, env, false)

	// 5) Stats reflect the one insert.
	do(t, client, http.MethodGet, base+"/v1/filters/"+name, nil, http.StatusOK)

	// 6) Delete → 204.
	do(t, client, http.MethodDelete, base+"/v1/filters/"+name, nil, http.StatusNoContent)

	// 7) Stats on the deleted filter → 404.
	do(t, client, http.MethodGet, base+"/v1/filters/"+name, nil, http.StatusNotFound)
}

// do performs one request, asserts the status, and decodes the envelope (empty
// for 204). body is JSON-encoded when non-nil.
func do(t *testing.T, c *http.Client, method, url string, body any, wantStatus int) envelope {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("%s %s: new request: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want status %d, got %d — body: %s", method, url, wantStatus, resp.StatusCode, raw)
	}
	if resp.StatusCode == http.StatusNoContent || len(raw) == 0 {
		return envelope{}
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("%s %s: decode envelope: %v — body: %s", method, url, err, raw)
	}
	if env.CorrelationID == "" {
		t.Errorf("%s %s: missing correlation_id in envelope", method, url)
	}
	return env
}

func assertExists(t *testing.T, env envelope, want bool) {
	t.Helper()
	var d struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal(env.Data, &d); err != nil {
		t.Fatalf("decode exists data: %v", err)
	}
	if d.Exists != want {
		t.Fatalf("exists = %v, want %v", d.Exists, want)
	}
}
