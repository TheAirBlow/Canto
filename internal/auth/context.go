package auth

import (
	"context"

	"Canto/internal/db"
)

// ctxKey is an unexported type to avoid context key collisions across packages.
type ctxKey int

// userCtxKey is the context key the auth middleware stores the authenticated user under.
const userCtxKey ctxKey = iota

// ContextWithUser returns a copy of ctx carrying user.
func ContextWithUser(ctx context.Context, user db.User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

// UserFromContext returns the authenticated user stored on ctx, if any.
func UserFromContext(ctx context.Context) (db.User, bool) {
	user, ok := ctx.Value(userCtxKey).(db.User)
	return user, ok
}
