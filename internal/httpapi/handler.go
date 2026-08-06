// Package httpapi serves the public-facing site: the post index at "/" and
// individual posts at "/{slug}", rendered through theme.Renderer, plus
// static theme assets under "/assets/" (assets.go). This package only ever
// reads published content (content.ListPublished/content.GetPublished) —
// see design.md §3 on the public/admin read split; drafts are never
// reachable here.
package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/storage"
	"github.com/nori-kamiya/xordpress/internal/theme"
)

// Handler serves the public site: GET/HEAD "/" renders the published-post
// index (theme "index"), and GET/HEAD "/{slug}" renders a single published
// post (theme "post"), 404-ing if the slug does not exist or resolves to a
// draft. It is registered as a catch-all "/" pattern (design.md §5), which
// http.ServeMux matches for every method — so any method other than
// GET/HEAD is rejected here explicitly with 405.
func Handler(db *sql.DB, renderer *theme.Renderer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		slug := strings.TrimPrefix(r.URL.Path, "/")
		if slug == "" {
			serveIndex(w, r, db, renderer)
			return
		}
		servePost(w, r, db, renderer, slug)
	})
}

// serveIndex renders the "index" theme template with the current site
// settings and the list of published posts.
func serveIndex(w http.ResponseWriter, r *http.Request, db *sql.DB, renderer *theme.Renderer) {
	posts, err := content.ListPublished(db)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	site, err := content.GetSiteSettings(db)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"site":  siteBindings(site),
		"posts": postsBindings(posts),
	}

	out, err := renderer.Render(r.Context(), "index", data)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeHTML(w, out)
}

// servePost renders the "post" theme template for the published post
// identified by slug, or 404s if it does not exist (including when it
// exists only as a draft — content.GetPublished never returns those).
func servePost(w http.ResponseWriter, r *http.Request, db *sql.DB, renderer *theme.Renderer, slug string) {
	p, err := content.GetPublished(db, slug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	site, err := content.GetSiteSettings(db)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"site": siteBindings(site),
		"post": postBindings(p),
	}

	out, err := renderer.Render(r.Context(), "post", data)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeHTML(w, out)
}

// siteBindings converts content.SiteSettings into the map[string]any shape
// theme.Renderer.Render requires (see theme.toBindings).
func siteBindings(s content.SiteSettings) map[string]any {
	return map[string]any{"name": s.Name}
}

// postBindings converts a single content.Post into Liquid-friendly bindings
// matching the field names used by the starter theme templates (slug,
// title, body).
func postBindings(p content.Post) map[string]any {
	return map[string]any{
		"slug":  p.Slug,
		"title": p.Title,
		"body":  p.Body,
	}
}

// postsBindings converts a slice of content.Post into the []map[string]any
// shape the "index" template iterates over.
func postsBindings(posts []content.Post) []map[string]any {
	out := make([]map[string]any, len(posts))
	for i, p := range posts {
		out[i] = postBindings(p)
	}
	return out
}

// writeHTML writes out as an HTML response body with a 200 status.
func writeHTML(w http.ResponseWriter, out string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out))
}
