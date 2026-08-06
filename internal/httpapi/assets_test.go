package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/httpapi"
	"github.com/nori-kamiya/xordpress/internal/theme"
)

// muxWithAssets registers AssetsHandler under the same method-scoped
// pattern main.go uses (design.md §5), so ServeMux's own path
// cleaning/redirect behavior is exercised exactly as in production.
func muxWithAssets(renderer *theme.Renderer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", httpapi.AssetsHandler(renderer))
	return mux
}

func TestAssetsHandler_ServesValidAsset(t *testing.T) {
	handler := muxWithAssets(theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodGet, "/assets/style.css")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "font-family") {
		t.Errorf("expected style.css content, got: %s", rec.Body.String())
	}
}

func TestAssetsHandler_UnknownAssetReturns404(t *testing.T) {
	handler := muxWithAssets(theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodGet, "/assets/does-not-exist.css")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

// TestAssetsHandler_RejectsPercentEncodedTraversal exercises the
// path-traversal guard end to end. A raw "../" in the request path is
// normalized and 307-redirected by http.ServeMux before ever reaching the
// handler (see design.md §5), so it never gets a chance to exercise the
// guard. A percent-encoded traversal string reaches AssetsHandler with
// r.URL.Path already decoded to contain "..", because ServeMux's own
// redirect check operates on the *escaped* path (which still reads
// "%2e%2e%2f...") — so the guard inside theme.ResolveAsset is what actually
// rejects it here.
func TestAssetsHandler_RejectsPercentEncodedTraversal(t *testing.T) {
	handler := muxWithAssets(theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodGet, "/assets/%2e%2e%2f%2e%2e%2fetc%2fpasswd")

	if rec.Code == http.StatusOK {
		t.Fatalf("expected the traversal attempt to be rejected, got 200 body=%s", rec.Body.String())
	}
}
