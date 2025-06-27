package http

import "context"

type contextKey int

var userKey contextKey

func contextWithUserID(ctx context.Context, userID []byte) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

func userIDFromContext(ctx context.Context) ([]byte, bool) {
	id, ok := ctx.Value(userKey).([]byte)
	return id, ok
}
