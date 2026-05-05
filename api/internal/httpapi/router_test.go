package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"accordli.com/analyze-ai/api/internal/infra/auth"
	"accordli.com/analyze-ai/api/internal/infra/observability"
)

func TestHealthHandler(t *testing.T) {
	deps := &Deps{
		Auth: auth.NewHardcoded(
			uuid.New(), uuid.New(), uuid.New(), "smoke@test",
		),
		Log:     observability.NewStdout(),
		Version: "test-sha",
	}
	srv := httptest.NewServer(NewRouter(deps))
	defer srv.Close()

	for _, path := range []string{"/health", "/api/health"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request %s: %v", path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, resp.StatusCode)
		}
		var body struct {
			OK      bool   `json:"ok"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		resp.Body.Close()
		if !body.OK {
			t.Fatalf("%s: ok = false", path)
		}
		if body.Version != "test-sha" {
			t.Fatalf("%s: version = %q, want test-sha", path, body.Version)
		}
	}
}
