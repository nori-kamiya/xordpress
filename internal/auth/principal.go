package auth

import "context"

// Principal identifies the authenticated user attached to a request's
// context by AuthMiddleware, carrying the Role needed for capability checks
// alongside the UserID needed for ownership checks.
type Principal struct {
	UserID int64
	Role   Role
}

type principalContextKey struct{}

// WithPrincipal returns a copy of ctx carrying p, retrievable via
// PrincipalFrom.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// PrincipalFrom returns the Principal stored in ctx by AuthMiddleware, if
// any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok
}
