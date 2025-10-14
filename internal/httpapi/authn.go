package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"qazna.org/internal/auth"
)

const (
	authHeader = "Authorization"
	bearer     = "Bearer "
)

var publicPaths = []string{
	"/v1/auth/token",
	"/metrics",
	"/healthz",
	"/readyz",
	"/openapi.yaml",
	"/",
	"/admin/dashboard",
	"/banks/dashboard",
}
var publicPrefixes = []string{
	"/assets/",
}

func (a *API) withAuth(next http.Handler) http.Handler {
	if a == nil || a.auth == nil || !a.auth.SupportsTokens() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token, err := extractBearerToken(r.Header.Get(authHeader))
		if err != nil {
			respondError(w, http.StatusUnauthorized, err.Error())
			return
		}

		principal, err := a.auth.AuthenticateToken(r.Context(), token)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrInvalidToken):
				respondError(w, http.StatusUnauthorized, "invalid token")
			default:
				respondError(w, http.StatusInternalServerError, "authentication error")
			}
			return
		}

		ctx := auth.ContextWithPrincipal(r.Context(), principal)
		ctx = auth.ContextWithToken(ctx, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) requirePermission(ctx context.Context, perm string) error {
	if a == nil || a.auth == nil {
		return nil
	}
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok {
		return auth.ErrUnauthorized
	}
	if !principal.HasPermission(perm) {
		return auth.ErrUnauthorized
	}
	return nil
}

func extractBearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", errors.New("missing bearer token")
	}
	if !strings.HasPrefix(strings.ToLower(header), strings.ToLower(bearer)) {
		return "", errors.New("invalid authorization scheme")
	}
	token := strings.TrimSpace(header[len(bearer):])
	if token == "" {
		return "", errors.New("missing bearer token")
	}
	return token, nil
}

func isPublicPath(path string) bool {
	for _, p := range publicPaths {
		if path == p {
			return true
		}
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
