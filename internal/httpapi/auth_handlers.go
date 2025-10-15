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

const tokenTTL = 15 * time.Minute

func (a *API) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
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

	token, err := auth.GenerateToken(user, roles, tokenTTL)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "token generation failed")
		return
	}

	expiresAt := time.Now().UTC().Add(tokenTTL)
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
