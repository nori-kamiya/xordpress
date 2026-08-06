package adminui

import (
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

// distPrefix is the path prefix under which the admin UI is served.
const distPrefix = "/admin/"

// Handler serves the embedded admin UI (FS's dist/ subtree) as a
// single-page application: static files are served as-is, and any subpath
// that does not resolve to a file falls back to index.html so client-side
// routing can take over. Requests to /admin (no trailing slash) are
// redirected to /admin/.
func Handler() http.Handler {
	sub, err := fs.Sub(FS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin" {
			http.Redirect(w, r, distPrefix, http.StatusMovedPermanently)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, distPrefix)
		if name == "" {
			name = "index.html"
		}

		if _, err := fs.Stat(sub, name); err != nil {
			r2 := new(http.Request)
			*r2 = *r
			u := new(url.URL)
			*u = *r.URL
			u.Path = "/"
			r2.URL = u
			fileServer.ServeHTTP(w, r2)
			return
		}

		http.StripPrefix(distPrefix, fileServer).ServeHTTP(w, r)
	})
}
