package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// allowedTopLevelAPIPatterns are the only /api patterns app.go may register
// directly on the top-level mux, bypassing CSRFMiddleware and AuthMiddleware
// (design.md §6.3). POST /api/setup and POST /api/session run before any
// session exists, so they cannot go through AuthMiddleware; POST
// /api/session/mfa (added by T12) is the same case for the second half of
// the 2FA login flow. "/api/" is the catch-all that wraps everything else
// in both middlewares via adminapi.Handler.
//
// /admin/, GET /assets/, and / are deliberately NOT in this set: they are
// legitimately unwrapped public/UI surfaces, not part of the CSRF/Auth gap
// this invariant guards against.
var allowedTopLevelAPIPatterns = map[string]bool{
	"POST /api/setup":       true,
	"POST /api/session":     true,
	"POST /api/session/mfa": true,
	"/api/":                 true,
}

// TestTopLevelMux_OnlyRegistersAllowedAPIPatterns statically parses app.go
// and asserts that every /api pattern passed to mux.Handle or
// mux.HandleFunc is in allowedTopLevelAPIPatterns. Go's http.ServeMux picks
// the most specific matching pattern, so a stray direct registration like
// mux.Handle("/api/foo", h) would pre-empt the wrapped "/api/" catch-all
// and silently bypass CSRFMiddleware and AuthMiddleware for that path. This
// is a subset check, not an exact-set check, so it stays green as future
// tasks (e.g. T12's /api/session/mfa) add already-allowed patterns.
func TestTopLevelMux_OnlyRegistersAllowedAPIPatterns(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("parse app.go: %v", err)
	}

	var found int
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		pattern, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote pattern literal %s: %v", lit.Value, err)
		}
		if !isAPIPattern(pattern) {
			return true
		}
		found++
		if !allowedTopLevelAPIPatterns[pattern] {
			t.Errorf(
				"app.go registers %q directly on the top-level mux, but it is not in allowedTopLevelAPIPatterns; "+
					"a more specific pattern than \"/api/\" pre-empts Go ServeMux's wrapped catch-all and would "+
					"bypass CSRFMiddleware/AuthMiddleware for this path — route it through adminapi.Handler's "+
					"route table instead, or add it here only if it must run before a session exists",
				pattern,
			)
		}
		return true
	})

	if found == 0 {
		t.Fatal("found no /api mux registrations in app.go; the AST walk may be broken")
	}
}

func isAPIPattern(pattern string) bool {
	if len(pattern) >= 4 && pattern[:4] == "/api" {
		return true
	}
	for i, c := range pattern {
		if c == ' ' {
			return isAPIPattern(pattern[i+1:])
		}
	}
	return false
}
