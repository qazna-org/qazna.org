package auth

import "context"

type principalContextKey struct{}
type tokenContextKey struct{}

// ContextWithPrincipal attaches the authenticated principal to the context.
func ContextWithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, &principal)
}

// PrincipalFromContext extracts the authenticated principal from the context.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	v, ok := ctx.Value(principalContextKey{}).(*Principal)
	if !ok || v == nil {
		return Principal{}, false
	}
	return *v, true
}

// ContextWithToken stores the raw bearer token inside the context.
func ContextWithToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, tokenContextKey{}, token)
}

// TokenFromContext returns the bearer token if it was previously attached.
func TokenFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(tokenContextKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
