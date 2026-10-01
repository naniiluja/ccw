package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRootRedirectsToTheUI(t *testing.T) {
	h := NewWithAuth(openStore(t), nil, authConfig())
	rec := send(h, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/ui/" {
		t.Fatalf("GET /: code=%d loc=%q, want 302 -> /ui/", rec.Code, rec.Header().Get("Location"))
	}
}

func TestUIIsPublicAndTheOtherRoutesKeepTheirGates(t *testing.T) {
	h := NewWithAuth(openStore(t), nil, authConfig())
	cases := []struct {
		method, path string
		want         int
	}{
		{"GET", "/ui/", 0}, // 200 once built, 503 before; never a login redirect or 401
		{"GET", "/ui/accounts", 0},
		{"GET", "/ui", http.StatusTemporaryRedirect},
		{"POST", "/ui/", http.StatusMethodNotAllowed},
		{"GET", "/v1/models", http.StatusUnauthorized},
		{"GET", "/api/accounts", http.StatusUnauthorized},
	}
	for _, c := range cases {
		rec := send(h, httptest.NewRequest(c.method, "http://127.0.0.1"+c.path, nil))
		if c.want == 0 {
			if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
				t.Errorf("%s %s: code=%d, want 200 or 503", c.method, c.path, rec.Code)
			}
			continue
		}
		if rec.Code != c.want {
			t.Errorf("%s %s: code=%d, want %d", c.method, c.path, rec.Code, c.want)
		}
	}
}

func TestUIGoesThroughTheSecurityHeaders(t *testing.T) {
	h := NewWithAuth(openStore(t), nil, authConfig())
	rec := send(h, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/ui/", nil))
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("/ui/ skipped guardRequest")
	}
}
