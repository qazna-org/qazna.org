package httpapi

import (
	"net/http"
	"strings"
	"time"

	"qazna.org/internal/audit"
	"qazna.org/internal/auth"
)

type tokenRequest struct {
	User  string   `json:"user"`
	Roles []string `json:"roles"`
}

type tokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type oauthAuthorizeRequest struct {
	ClientID            string   `json:"client_id"`
	RedirectURI         string   `json:"redirect_uri"`
	CodeChallenge       string   `json:"code_challenge"`
	CodeChallengeMethod string   `json:"code_challenge_method"`
	User                string   `json:"user"`
	Roles               []string `json:"roles"`
}

type authCodeResponse struct {
	Code        string    `json:"code"`
	RedirectURI string    `json:"redirect_uri"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type oauthTokenRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
}

const tokenTTL = 15 * time.Minute

func (a *API) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if a.auth == nil {
		writeError(w, r, http.StatusNotImplemented, "authentication service unavailable")
		return
	}

	var req tokenRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	user := strings.TrimSpace(req.User)
	if user == "" {
		writeError(w, r, http.StatusBadRequest, "user is required")
		return
	}
	roles := make([]string, 0, len(req.Roles))
	for _, role := range req.Roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		writeError(w, r, http.StatusBadRequest, "roles are required")
		return
	}

	token, expiresAt, err := a.auth.GenerateToken(r.Context(), user, roles, tokenTTL)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "token generation failed")
		return
	}

	fields := map[string]any{
		"user":       user,
		"roles":      roles,
		"expires_at": expiresAt.Format(time.RFC3339),
	}
	_ = audit.LogEvent(r.Context(), "auth.token.issued", fields)

	writeJSON(w, http.StatusOK, tokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	})
}

func (a *API) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if a.auth == nil {
		writeError(w, r, http.StatusNotImplemented, "authentication service unavailable")
		return
	}

	var req oauthAuthorizeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	roles := make([]string, 0, len(req.Roles))
	for _, role := range req.Roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		roles = append(roles, role)
	}

	code, err := a.auth.IssueAuthCode(r.Context(), auth.AuthCodeRequest{
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		UserID:              req.User,
		Roles:               roles,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	fields := map[string]any{
		"user":                  strings.TrimSpace(req.User),
		"client_id":             strings.TrimSpace(req.ClientID),
		"code_challenge_method": strings.TrimSpace(req.CodeChallengeMethod),
	}
	_ = audit.LogEvent(r.Context(), "auth.oauth.code.issue", fields)

	writeJSON(w, http.StatusOK, authCodeResponse{
		Code:        code.Code,
		RedirectURI: code.RedirectURI,
		ExpiresAt:   code.ExpiresAt,
	})
}

func (a *API) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if a.auth == nil {
		writeError(w, r, http.StatusNotImplemented, "authentication service unavailable")
		return
	}
	var req oauthTokenRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	token, expiresAt, err := a.auth.ExchangeAuthCode(r.Context(), auth.AuthCodeExchangeRequest{
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		Code:         req.Code,
		CodeVerifier: req.CodeVerifier,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	})
}
