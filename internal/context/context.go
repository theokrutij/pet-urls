package context

import "context"

type contextKey struct{}

var requestIDKey contextKey
var userKey contextKey

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey).(string)
	return id, ok
}

func ContextWithUserID(ctx context.Context, userID []byte) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

func UserIDFromContext(ctx context.Context) ([]byte, bool) {
	id, ok := ctx.Value(userKey).([]byte)
	return id, ok
}
