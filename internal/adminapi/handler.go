// Package adminapi: internal routing assembly (design.md §4, §5).
package adminapi

import (
	"database/sql"
	"net/http"
)

// Handler assembles the authenticated admin API from the routes() table
// (design.md §4), wrapping each entry's handler with requireCap so it only
// runs for a Principal (attached to the request context by
// auth.AuthMiddleware) whose Role has the declared Capability. It registers
// full path+method patterns for every /api/* endpoint except POST
// /api/setup and POST /api/session, which are registered individually at
// the top-level mux (unauthenticated) and are not part of this handler.
//
// contentDB is used for the posts and settings endpoints; authDB is used
// for DELETE /api/session and to resolve the authenticated user's id for
// POST /api/posts.
//
// Handler returns http.Handler rather than *http.ServeMux so callers cannot
// later add routes via mux.Handle that bypass requireCap.
func Handler(contentDB *sql.DB, authDB *sql.DB) http.Handler {
	mux := http.NewServeMux()
	for _, rt := range routes(contentDB, authDB) {
		mux.Handle(rt.pattern, requireCap(rt.cap, rt.h))
	}
	return mux
}
