package httpapi

import (
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/nori-kamiya/xordpress/internal/theme"
)

// assetsPrefix is the path prefix stripped before resolving a request path
// against the active theme's assets/ directory.
const assetsPrefix = "/assets/"

// AssetsHandler serves static theme assets under GET /assets/, resolving
// each request path through theme.Renderer.ResolveAsset (design.md §5).
// It is registered under the method-scoped pattern "GET /assets/"
// (design.md §5), so http.ServeMux itself rejects other methods before this
// handler runs. Path-traversal attempts are rejected by ResolveAsset — see
// its doc comment.
func AssetsHandler(renderer *theme.Renderer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, assetsPrefix)

		f, err := renderer.ResolveAsset(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, f)
	})
}
