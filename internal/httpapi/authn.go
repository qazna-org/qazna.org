package httpapi

import (
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
	"/v1/auth/oauth/token",
	"/v1/auth/oauth/authorize",
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
	if a == nil || a.auth == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token, err := extractBearerToken(r.Header.Get(authHeader))
		if err != nil {
			setWWWAuthenticate(w, "invalid_request", err.Error())
			writeError(w, r, http.StatusUnauthorized, err.Error())
			return
		}

		claims, err := a.auth.ParseAndValidate(r.Context(), token)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				setWWWAuthenticate(w, "invalid_token", "token validation failed")
				writeError(w, r, http.StatusUnauthorized, "invalid token")
				return
			}
			setWWWAuthenticate(w, "server_error", "authentication validation failed")
			writeError(w, r, http.StatusInternalServerError, "authentication error")
			return
		}

		ctx := auth.ContextWithUser(r.Context(), claims.Subject, claims.Roles)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole enforces that the request context contains at least one of the specified roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(roles) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := auth.UserIDFromContext(r.Context()); !ok {
				setWWWAuthenticate(w, "invalid_token", "missing authentication context")
				writeError(w, r, http.StatusUnauthorized, "authentication required")
				return
			}
			for _, role := range roles {
				if auth.HasRole(r.Context(), role) {
					next.ServeHTTP(w, r)
					return
				}
			}
			setWWWAuthenticate(w, "insufficient_scope", "missing required role")
			writeError(w, r, http.StatusForbidden, "missing required role")
		})
	}
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

func setWWWAuthenticate(w http.ResponseWriter, code, desc string) {
	params := []string{}
	if code != "" {
		params = append(params, `error="`+code+`"`)
	}
	if desc != "" {
		params = append(params, `error_description="`+desc+`"`)
	}
	value := "Bearer realm=\"qazna\""
	if len(params) > 0 {
		value += ", " + strings.Join(params, ", ")
	}
	w.Header().Set("WWW-Authenticate", value)
}
