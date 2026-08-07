// Package adminapi: posts CRUD handlers (design.md §4).
package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// maxRequestBodyBytes is the overall request body cap enforced via
// http.MaxBytesReader (design.md §4). Exceeding it yields 413.
const maxRequestBodyBytes = 2 << 20 // 2MB

// maxBodyFieldBytes is the per-field cap on the post body content
// (design.md §4's validation rules), independent of maxRequestBodyBytes.
const maxBodyFieldBytes = 1 << 20 // 1MB

// maxTitleLen is the upper bound on the title field (design.md §4).
const maxTitleLen = 200

// defaultLimit/maxLimit are the GET /api/posts pagination defaults
// (design.md §4): limit defaults to 20, capped at 100.
const (
	defaultLimit = 20
	maxLimit     = 100
)

// slugPattern matches the required slug shape (design.md §4): lowercase
// kebab-case only.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ifMatchPattern matches the required If-Match header shape: a
// quote-wrapped integer version, e.g. `"3"` (design.md §4).
var ifMatchPattern = regexp.MustCompile(`^"([0-9]+)"$`)

// reservedSlugs are slugs that would collide with top-level routes and are
// therefore rejected outright (design.md §4).
var reservedSlugs = map[string]bool{
	"api":    true,
	"admin":  true,
	"assets": true,
}

// postRequest is the JSON body shape shared by POST /api/posts and
// PUT /api/posts/{id}.
type postRequest struct {
	Title  string `json:"title"`
	Slug   string `json:"slug"`
	Body   string `json:"body"`
	Status string `json:"status"`
}

// postResponse is the full post representation returned by
// POST/GET/PUT /api/posts(/{id}) (design.md §4).
type postResponse struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Status      string `json:"status"`
	Version     int64  `json:"version"`
	AuthorID    int64  `json:"author_id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	PublishedAt *int64 `json:"published_at"`
}

// postSummary is the reduced representation used for list items
// (design.md §4: {id,slug,title,status,version,updated_at}).
type postSummary struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Version   int64  `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

// postListResponse is the GET /api/posts envelope (design.md §4).
type postListResponse struct {
	Items []postSummary `json:"items"`
	Total int           `json:"total"`
}

func toPostResponse(p *content.Post) postResponse {
	return postResponse{
		ID:          p.ID,
		Slug:        p.Slug,
		Title:       p.Title,
		Body:        p.Body,
		Status:      p.Status,
		Version:     p.Version,
		AuthorID:    p.AuthorID,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		PublishedAt: p.PublishedAt,
	}
}

func toPostSummary(p *content.Post) postSummary {
	return postSummary{
		ID:        p.ID,
		Slug:      p.Slug,
		Title:     p.Title,
		Status:    p.Status,
		Version:   p.Version,
		UpdatedAt: p.UpdatedAt,
	}
}

// validatePostFields applies the format-validation rules from design.md
// §4 (title/slug/body/status) shared by create and update. It returns a
// non-empty message describing the first violation found, or "" if valid.
func validatePostFields(req postRequest) string {
	if len(req.Title) < 1 || len(req.Title) > maxTitleLen {
		return "invalid title"
	}
	if len(req.Body) < 1 || len(req.Body) > maxBodyFieldBytes {
		return "invalid body"
	}
	if !slugPattern.MatchString(req.Slug) || reservedSlugs[req.Slug] {
		return "invalid slug"
	}
	if req.Status != "draft" && req.Status != "published" {
		return "invalid status"
	}
	return ""
}

// decodePostBody enforces the overall request-size cap and decodes the
// JSON body, distinguishing a too-large body (413) from a malformed one
// (400).
func decodePostBody(w http.ResponseWriter, r *http.Request) (postRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req postRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return postRequest{}, false
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return postRequest{}, false
	}
	return req, true
}

// parseIfMatch extracts and validates the required If-Match header
// (design.md §4): missing or unparsable yields ok=false.
func parseIfMatch(r *http.Request) (version int64, ok bool) {
	header := r.Header.Get("If-Match")
	if header == "" {
		return 0, false
	}
	matches := ifMatchPattern.FindStringSubmatch(header)
	if matches == nil {
		return 0, false
	}
	v, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parsePostID extracts and parses the {id} path value, shared by
// GET/PUT/DELETE /api/posts/{id}.
func parsePostID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// currentUserID resolves the requesting user's id from the session cookie
// (design.md §4; mirrors Logout's defensive re-derivation in session.go so
// the handler behaves correctly under direct test too, without relying on
// authMiddleware having populated request context).
func currentUserID(r *http.Request, authDB *sql.DB) (int64, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, false
	}
	return auth.ValidateSession(r.Context(), authDB, cookie.Value)
}

// currentPrincipal resolves the requesting user's Principal (UserID and
// Role), preferring the value AuthMiddleware attached to the request
// context. Falls back to re-deriving it from the session cookie via
// auth.ValidateSessionWithRole so handlers still behave correctly under
// direct tests that bypass AuthMiddleware (design.md §4).
func currentPrincipal(r *http.Request, authDB *sql.DB) (auth.Principal, bool) {
	if p, ok := auth.PrincipalFrom(r.Context()); ok {
		return p, true
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return auth.Principal{}, false
	}
	return auth.ValidateSessionWithRole(r.Context(), authDB, cookie.Value)
}

// ListPosts returns the handler for GET /api/posts (design.md §4).
// Supports ?status=&limit=&offset= (limit default 20/cap 100, offset
// default 0; malformed values -> 400). total reflects the count after the
// optional status filter.
func ListPosts(contentDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, offset, ok := parseListParams(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid limit or offset")
			return
		}

		all, err := content.List(contentDB, storage.PostFilter{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		status := r.URL.Query().Get("status")
		filtered := all
		if status != "" {
			filtered = make([]*content.Post, 0, len(all))
			for _, p := range all {
				if p.Status == status {
					filtered = append(filtered, p)
				}
			}
		}

		total := len(filtered)
		page := paginate(filtered, offset, limit)

		items := make([]postSummary, len(page))
		for i, p := range page {
			items[i] = toPostSummary(p)
		}

		writeJSON(w, http.StatusOK, postListResponse{Items: items, Total: total})
	})
}

// paginate slices posts to the [offset, offset+limit) window, clamping to
// the available range.
func paginate(posts []*content.Post, offset, limit int) []*content.Post {
	if offset >= len(posts) {
		return []*content.Post{}
	}
	end := offset + limit
	if end > len(posts) {
		end = len(posts)
	}
	return posts[offset:end]
}

// parseListParams parses and validates the limit/offset query parameters
// (design.md §4). Absent values fall back to the defaults; malformed or
// negative values are rejected; limit above maxLimit is capped.
func parseListParams(r *http.Request) (limit, offset int, ok bool) {
	limit = defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		limit = n
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	offset = 0
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, 0, false
		}
		offset = n
	}

	return limit, offset, true
}

// CreatePost returns the handler for POST /api/posts (design.md §4). It
// validates the request, resolves the authenticated user as author_id via
// the session cookie, and creates the post via content.Create. A slug
// collision maps to 409.
func CreatePost(contentDB *sql.DB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePostBody(w, r)
		if !ok {
			return
		}

		if msg := validatePostFields(req); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}

		principal, ok := currentPrincipal(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if req.Status == "published" && !principal.Role.Has(auth.CapPostsPublish) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		p, err := content.Create(contentDB, content.CreateInput{
			Slug:     req.Slug,
			Title:    req.Title,
			Body:     req.Body,
			Status:   req.Status,
			AuthorID: principal.UserID,
		})
		if err != nil {
			if errors.Is(err, storage.ErrSlugConflict) {
				writeError(w, http.StatusConflict, "slug already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusCreated, toPostResponse(p))
	})
}

// GetPost returns the handler for GET /api/posts/{id} (design.md §4).
func GetPost(contentDB *sql.DB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePostID(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		p, err := content.Get(contentDB, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, toPostResponse(p))
	})
}

// UpdatePost returns the handler for PUT /api/posts/{id} (design.md §4).
// Requires If-Match: "<version>" (missing/unparsable -> 400); maps
// ErrNotFound -> 404, ErrSlugConflict -> 409, ErrVersionMismatch -> 412.
func UpdatePost(contentDB *sql.DB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePostID(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		principal, ok := currentPrincipal(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		existing, err := content.Get(contentDB, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if existing.AuthorID != principal.UserID && !principal.Role.Has(auth.CapPostsEditOthers) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		version, ok := parseIfMatch(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "missing or invalid If-Match header")
			return
		}

		req, ok := decodePostBody(w, r)
		if !ok {
			return
		}

		if msg := validatePostFields(req); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		if req.Status == "published" && !principal.Role.Has(auth.CapPostsPublish) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		p, err := content.Update(contentDB, id, version, content.UpdateInput{
			Slug:   req.Slug,
			Title:  req.Title,
			Body:   req.Body,
			Status: req.Status,
		})
		if err != nil {
			switch {
			case errors.Is(err, storage.ErrNotFound):
				writeError(w, http.StatusNotFound, "post not found")
			case errors.Is(err, storage.ErrSlugConflict):
				writeError(w, http.StatusConflict, "slug already exists")
			case errors.Is(err, storage.ErrVersionMismatch):
				writeError(w, http.StatusPreconditionFailed, "version mismatch")
			default:
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		writeJSON(w, http.StatusOK, toPostResponse(p))
	})
}

// DeletePost returns the handler for DELETE /api/posts/{id} (design.md
// §4). Requires If-Match: "<version>" (missing/unparsable -> 400). Since
// storage.DeletePost cannot itself distinguish "no such id" from "version
// mismatch" (both yield 0 rows affected), existence is checked first via
// content.Get so the two can be reported as 404 vs 412 respectively.
func DeletePost(contentDB *sql.DB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsePostID(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		principal, ok := currentPrincipal(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		version, ok := parseIfMatch(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "missing or invalid If-Match header")
			return
		}

		existing, err := content.Get(contentDB, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if existing.AuthorID != principal.UserID && !principal.Role.Has(auth.CapPostsEditOthers) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		if err := content.Delete(contentDB, id, version); err != nil {
			if errors.Is(err, storage.ErrVersionMismatch) {
				writeError(w, http.StatusPreconditionFailed, "version mismatch")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// writeJSON writes v as a JSON response body with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
