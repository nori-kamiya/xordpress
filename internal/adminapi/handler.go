// Package adminapi: internal routing assembly (design.md §4, §5).
package adminapi

import (
	"database/sql"
	"net/http"
)

// Handler assembles the internal *http.ServeMux for the authenticated
// admin API (design.md §5). It registers full path+method patterns for
// every /api/* endpoint except POST /api/setup and POST /api/session,
// which are registered individually at the top-level mux (unauthenticated)
// and are not part of this handler.
//
// contentDB is used for the posts and settings endpoints; authDB is used
// for DELETE /api/session and to resolve the authenticated user's id for
// POST /api/posts.
func Handler(contentDB *sql.DB, authDB *sql.DB) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("GET /api/posts", ListPosts(contentDB))
	mux.Handle("POST /api/posts", CreatePost(contentDB, authDB))
	mux.Handle("GET /api/posts/{id}", GetPost(contentDB))
	mux.Handle("PUT /api/posts/{id}", UpdatePost(contentDB))
	mux.Handle("DELETE /api/posts/{id}", DeletePost(contentDB))

	mux.Handle("DELETE /api/session", Logout(authDB))

	mux.Handle("PUT /api/settings", UpdateSettings(contentDB))

	return mux
}
