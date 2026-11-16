package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// get runs one request against the asset handler.
func get(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	handler, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func TestRootServesTheAppDocument(t *testing.T) {
	recorder := get(t, http.MethodGet, "/")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type = %q", got)
	}

	body := recorder.Body.String()
	// The document has to mount the app, load its bundle, and name its icon.
	for _, want := range []string{`id="root"`, `/app.js`, `/app.css`, `/favicon.svg`} {
		if !strings.Contains(body, want) {
			t.Errorf("document does not contain %q", want)
		}
	}
}

func TestFaviconIsAnSVG(t *testing.T) {
	// A browser only uses an SVG icon when the sheet says so, and a wrong
	// content type makes some of them ignore it silently.
	recorder := get(t, http.MethodGet, "/favicon.svg")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), "<svg") {
		t.Errorf("favicon is not an svg: %.40q", body)
	}
	// The accent colour is what makes it recognisable at 16px.
	if !strings.Contains(body, "#3ecf8e") {
		t.Error("favicon does not use the accent colour")
	}
}

func TestAppDocumentIsNotCached(t *testing.T) {
	// The document names the current bundle, so a cached copy would keep
	// serving an old app after the binary is replaced.
	recorder := get(t, http.MethodGet, "/")

	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("cache control = %q, want no-cache", got)
	}
}

func TestBundleIsServed(t *testing.T) {
	cases := map[string]string{
		"/app.js":      "javascript",
		"/app.css":     "text/css",
		"/favicon.svg": "image/svg+xml",
	}
	for path, wantType := range cases {
		recorder := get(t, http.MethodGet, path)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, recorder.Code)
			continue
		}
		if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, wantType) {
			t.Errorf("%s: content type = %q, want something containing %q", path, got, wantType)
		}
		if recorder.Body.Len() == 0 {
			t.Errorf("%s: empty body", path)
		}
		// Embedded files never change inside a binary, so they may be cached.
		if got := recorder.Header().Get("Cache-Control"); !strings.Contains(got, "max-age") {
			t.Errorf("%s: cache control = %q, want a max-age", path, got)
		}
	}
}

func TestAppRoutesFallBackToTheDocument(t *testing.T) {
	// The app owns everything without a file extension, so a reload of a
	// deep link has to get the document rather than a 404.
	for _, path := range []string{"/nodes", "/nodes/web-1", "/anything/deep"} {
		recorder := get(t, http.MethodGet, path)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, recorder.Code)
			continue
		}
		if !strings.Contains(recorder.Body.String(), `id="root"`) {
			t.Errorf("%s: did not serve the app document", path)
		}
	}
}

func TestMissingAssetIsNotFound(t *testing.T) {
	// Answering a missing stylesheet with the HTML document would be worse
	// than a plain 404: the browser reports a parse error instead.
	recorder := get(t, http.MethodGet, "/missing.css")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}

func TestAPIPathsAreNeverSwallowed(t *testing.T) {
	// A typo in an API path must be visible as a 404, not hidden behind a
	// page of HTML.
	for _, path := range []string{"/api/v1/nope", "/api/", "/api/v1/nodes/x/y"} {
		recorder := get(t, http.MethodGet, path)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, recorder.Code)
		}
	}
}

func TestOtherMethodsAreRejected(t *testing.T) {
	recorder := get(t, http.MethodPost, "/")

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET, HEAD" {
		t.Errorf("Allow = %q", got)
	}
}

func TestHeadRequestHasNoBody(t *testing.T) {
	recorder := get(t, http.MethodHead, "/")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("HEAD returned %d bytes of body", recorder.Body.Len())
	}
}

func TestAssetName(t *testing.T) {
	assets := map[string]string{
		"/app.js":        "app.js",
		"/app.css":       "app.css",
		"/fonts/x.woff2": "fonts/x.woff2",
	}
	for path, want := range assets {
		got, ok := assetName(path)
		if !ok {
			t.Errorf("%s: not treated as an asset", path)
			continue
		}
		if got != want {
			t.Errorf("%s: name = %q, want %q", path, got, want)
		}
	}

	// No extension means a route for the app, not a file.
	for _, path := range []string{"/", "", "/nodes", "/nodes/web-1"} {
		if name, ok := assetName(path); ok {
			t.Errorf("%s: treated as asset %q", path, name)
		}
	}
}
