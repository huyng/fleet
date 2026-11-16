// Package web serves the single page app that the master shows.
//
// It serves files and nothing else: the bundle in dist/ is built from ui/ and
// baked into the binary, and the app fetches its data from the master's own
// JSON endpoints. Keeping the asset serving here means the master package
// only has to mount one handler.
package web

//go:generate sh -c "cd ../../ui && npm run build"

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed dist
var distFS embed.FS

// assetCacheSeconds is how long a browser may reuse the bundle. The files are
// embedded, so they only change when the binary does. The page itself is not
// cached, so a new binary is picked up on the next reload.
const assetCacheSeconds = 300

// IndexPath is the URL of the app itself.
const IndexPath = "/"

// indexFile is the document the app mounts into.
const indexFile = "dist/index.html"

// Handler serves the app and its assets.
//
// Every path outside /api/ falls back to index.html, so the app owns the
// routes it shows in the address bar. Paths under /api/ are never swallowed:
// they belong to the master, and a wrong one is a 404 rather than a page.
//
// Returns:
//
//	A handler for the whole site.
//
// Raises:
//
//	An error if the embedded bundle has no index.html, which means the binary
//	was built without running the frontend build.
func Handler() (http.Handler, error) {
	files, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, fmt.Errorf("opening embedded bundle: %w", err)
	}

	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil, fmt.Errorf("the embedded bundle has no index.html "+
			"(run the frontend build in ui/): %w", err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only GET and HEAD reach here; anything else is a mistake on the
		// caller's side.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			// Belongs to the master. Answering with the app here would hide a
			// typo in an API path behind a 200 and a page of HTML.
			http.NotFound(w, r)
			return
		}

		if name, ok := assetName(r.URL.Path); ok {
			if serveAsset(w, r, files, name) {
				return
			}
			// A missing asset is a 404: falling back to the page would hand a
			// browser HTML where it asked for CSS, which is harder to read
			// than a plain failure.
			http.NotFound(w, r)
			return
		}

		serveIndex(w, r, index)
	}), nil
}

// assetName decides whether a request path names a file in the bundle.
//
// Args:
//
//	urlPath: the request path.
//
// Returns:
//
//	The file name inside dist, and whether the path is an asset at all. A path
//	without a file extension is a route for the app, not an asset.
func assetName(urlPath string) (string, bool) {
	trimmed := strings.TrimPrefix(path.Clean(urlPath), "/")
	if trimmed == "" || trimmed == "." {
		return "", false
	}
	if !strings.Contains(path.Base(trimmed), ".") {
		return "", false
	}
	return trimmed, true
}

// serveAsset writes one file from the bundle.
//
// Args:
//
//	w: the response.
//	r: the request, used for conditional GET handling.
//	files: the embedded bundle.
//	name: file name inside the bundle.
//
// Returns:
//
//	True when the response was written, false when there is no such file.
func serveAsset(w http.ResponseWriter, r *http.Request, files fs.FS, name string) bool {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		return false
	}

	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", assetCacheSeconds))

	// ServeContent handles HEAD, ranges and conditional requests.
	http.ServeContent(w, r, name, startTime, bytes.NewReader(data))
	return true
}

// serveIndex writes the app document.
//
// Args:
//
//	w: the response.
//	r: the request.
//	index: the document.
func serveIndex(w http.ResponseWriter, r *http.Request, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The document names the current bundle, so caching it serves an old app
	// after an upgrade. It is small enough to send every time.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", startTime, bytes.NewReader(index))
}

// startTime is the modification time reported for embedded files. It has to
// be fixed, because those files are the same in every copy of the binary.
var startTime = time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
