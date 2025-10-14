package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"qazna.org/internal/auth"
)

type tokenRequest struct {
	GrantType    string `json:"grant_type"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken      string    `json:"access_token"`
	TokenType        string    `json:"token_type"`
	ExpiresIn        int64     `json:"expires_in"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshToken     string    `json:"refresh_token,omitempty"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitempty"`
}

func (a *API) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if a.auth == nil {
		respondError(w, http.StatusNotImplemented, "authentication service unavailable")
		return
	}

	var req tokenRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	grant := strings.TrimSpace(strings.ToLower(req.GrantType))
	switch grant {
	case "password":
		a.passwordGrant(w, r, &req)
	case "refresh_token":
		a.refreshGrant(w, r, &req)
	default:
		respondError(w, http.StatusBadRequest, "unsupported grant_type")
	}
}

func (a *API) passwordGrant(w http.ResponseWriter, r *http.Request, req *tokenRequest) {
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		respondError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	pair, principal, err := a.auth.IssueTokenPair(r.Context(), req.Email, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrUnauthorized) && !errors.Is(err, auth.ErrInvalidToken) {
			status = http.StatusInternalServerError
		}
		respondError(w, status, "invalid credentials")
		return
	}

	ctxWithPrincipal := auth.ContextWithPrincipal(r.Context(), principal)
	a.audit(ctxWithPrincipal, "auth.token.issue", "user", principal.User.ID, map[string]string{
		"grant_type": "password",
	})

	respondTokenPair(w, pair)
}

func (a *API) refreshGrant(w http.ResponseWriter, r *http.Request, req *tokenRequest) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		respondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}
	pair, principal, err := a.auth.RefreshTokenPair(r.Context(), req.RefreshToken)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrInvalidToken) {
			status = http.StatusInternalServerError
		}
		respondError(w, status, "invalid refresh token")
		return
	}

	ctxWithPrincipal := auth.ContextWithPrincipal(r.Context(), principal)
	a.audit(ctxWithPrincipal, "auth.token.refresh", "user", principal.User.ID, map[string]string{
		"grant_type": "refresh_token",
	})

	respondTokenPair(w, pair)
}

func respondTokenPair(w http.ResponseWriter, pair auth.TokenPair) {
	expiresIn := int64(time.Until(pair.AccessExpiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}
	resp := tokenResponse{
		AccessToken:      pair.AccessToken,
		TokenType:        "Bearer",
		ExpiresIn:        expiresIn,
		ExpiresAt:        pair.AccessExpiresAt.UTC(),
		RefreshToken:     pair.RefreshToken,
		RefreshExpiresAt: pair.RefreshExpiresAt.UTC(),
	}
	writeJSON(w, http.StatusOK, resp)
}
