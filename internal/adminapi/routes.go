// Package adminapi: declarative route table (design.md §4).
package adminapi

import (
	"database/sql"
	"net/http"

	"github.com/nori-kamiya/xordpress/internal/auth"
)

// routeEntry pairs a ServeMux pattern with the Capability required to reach
// its handler.
type routeEntry struct {
	pattern string
	cap     auth.Capability
	h       http.Handler
}

// route builds a routeEntry, panicking if cap is CapNone — every route must
// declare a real capability so requireCap always enforces something
// (design.md §4).
func route(pattern string, cap auth.Capability, h http.Handler) routeEntry {
	if cap == auth.CapNone {
		panic("adminapi: route " + pattern + " registered without a capability")
	}
	return routeEntry{pattern, cap, h}
}

// routes returns every authenticated admin API route paired with the
// capability required to access it.
func routes(contentDB, authDB *sql.DB) []routeEntry {
	return []routeEntry{
		route("GET /api/posts", auth.CapPostsRead, ListPosts(contentDB)),
		route("POST /api/posts", auth.CapPostsEditOwn, CreatePost(contentDB, authDB)),
		route("GET /api/posts/{id}", auth.CapPostsRead, GetPost(contentDB, authDB)),
		route("PUT /api/posts/{id}", auth.CapPostsEditOwn, UpdatePost(contentDB, authDB)),
		route("DELETE /api/posts/{id}", auth.CapPostsEditOwn, DeletePost(contentDB, authDB)),

		route("DELETE /api/session", auth.CapSelfManage, Logout(authDB)),
		route("GET /api/account", auth.CapSelfManage, GetAccount(contentDB, authDB)),
		route("PUT /api/account/password", auth.CapSelfManage, ChangePassword(contentDB, authDB)),
		route("GET /api/account/sessions", auth.CapSelfManage, ListSessions(authDB)),
		route("DELETE /api/account/sessions/{id}", auth.CapSelfManage, DeleteOwnSession(authDB)),

		route("GET /api/users", auth.CapUsersManage, ListUsers(contentDB)),
		route("POST /api/users", auth.CapUsersManage, CreateUser(contentDB)),
		route("GET /api/users/{id}", auth.CapUsersManage, GetUser(contentDB)),
		route("PATCH /api/users/{id}", auth.CapUsersManage, UpdateUser(contentDB, authDB)),
		route("DELETE /api/users/{id}", auth.CapUsersManage, DeleteUser(contentDB, authDB)),
		route("DELETE /api/users/{id}/mfa", auth.CapUsersManage, DeleteUserMFA(contentDB)),
		route("DELETE /api/users/{id}/sessions", auth.CapUsersManage, DeleteUserSessions(contentDB, authDB)),

		route("GET /api/settings", auth.CapSettingsManage, GetSettings(contentDB)),
		route("PUT /api/settings", auth.CapSettingsManage, UpdateSettings(contentDB)),
	}
}

// requireCap wraps next so it only runs when the request's Principal (set by
// auth.AuthMiddleware) has cap; otherwise it responds 401 (no Principal) or
// 403 (Principal lacks cap).
func requireCap(cap auth.Capability, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !p.Role.Has(cap) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}
