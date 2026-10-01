package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func built() fstest.MapFS {
	return fstest.MapFS{
		"index.html":        {Data: []byte("<html>app</html>")},
		"assets/app.abc.js": {Data: []byte("console.log(1)")},
		"secret.txt":        {Data: []byte("top")},
		"assets/dir.d/x.js": {Data: []byte("x")},
	}
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestUnbuiltUIAnswers503InVietnamese(t *testing.T) {
	h := handler(fstest.MapFS{".gitkeep": {}})
	for _, p := range []string{"/ui/", "/ui/accounts", "/ui/assets/a.js"} {
		rec := do(h, "GET", p)
		if rec.Code != 503 || !strings.Contains(rec.Body.String(), "UI chưa được build") {
			t.Errorf("%s: %d %q", p, rec.Code, rec.Body.String())
		}
	}
}

func TestBuiltUIServesIndexAssetsAndDeepLinks(t *testing.T) {
	h := handler(built())
	cases := []struct {
		method, path string
		code         int
		body, cache  string
	}{
		{"GET", "/ui/", 200, "app", "no-cache"},
		{"GET", "/ui/accounts", 200, "app", "no-cache"},
		{"GET", "/ui/accounts/12", 200, "app", "no-cache"},
		{"GET", "/ui/assets/app.abc.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"GET", "/ui/assets/missing.js", 404, "", ""},
		{"GET", "/ui/missing.png", 404, "", ""},
		{"GET", "/ui/secret.txt", 200, "top", ""},
		{"GET", "/ui/assets/dir.d", 404, "", ""},
		{"GET", "/ui/../secret.txt", 200, "top", ""},
		{"GET", "/ui/%2e%2e/%2e%2e/etc/passwd.txt", 404, "", ""},
		{"HEAD", "/ui/", 200, "", "no-cache"},
		{"POST", "/ui/", 405, "", ""},
		{"DELETE", "/ui/assets/app.abc.js", 405, "", ""},
	}
	for _, c := range cases {
		rec := do(h, c.method, c.path)
		if rec.Code != c.code {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, rec.Code, c.code)
			continue
		}
		if !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("%s %s: body %q lacks %q", c.method, c.path, rec.Body.String(), c.body)
		}
		if c.method == "HEAD" && rec.Body.Len() != 0 {
			t.Errorf("HEAD returned a body")
		}
		if got := rec.Header().Get("Cache-Control"); got != c.cache {
			t.Errorf("%s %s: Cache-Control %q, want %q", c.method, c.path, got, c.cache)
		}
	}
}

func TestHandlerEmbedsAnUnbuiltTree(t *testing.T) {
	if rec := do(Handler(), "GET", "/ui/"); rec.Code != 503 && rec.Code != 200 {
		t.Errorf("status %d", rec.Code)
	}
}
