package httpadapter

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"zeropad-backend/adapters/db"
	authsvc "zeropad-backend/services/auth"
)

// meResponse builds the shared user-representation payload used by
// /auth/me and the profile mutation endpoints. token is included only
// when non-empty (username rename issues a fresh one).
func meResponse(user db.User, token string) map[string]any {
	resp := map[string]any{
		"id":             user.ID,
		"username":       user.Username,
		"email_verified": user.EmailVerified,
	}
	if user.Email != "" {
		resp["email"] = user.Email
	}
	if user.WalletAddress != "" {
		resp["wallet_address"] = user.WalletAddress
	}
	if token != "" {
		resp["token"] = token
	}
	return resp
}

func (h *AuthHandler) handleUpdateUsername(w http.ResponseWriter, r *http.Request) {
	claims := authsvc.ClaimsFromContext(r.Context())
	user, ok, err := h.database.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid token"})
		return
	}

	var body struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
		return
	}

	updated, token, err := h.svc.UpdateUsername(r.Context(), user.ID, body.Username)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrDuplicateUsername):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already taken"})
		default:
			log.Printf("update username error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}

	writeJSON(w, http.StatusOK, meResponse(updated, token))
}

func (h *AuthHandler) handleUpdateEmail(w http.ResponseWriter, r *http.Request) {
	claims := authsvc.ClaimsFromContext(r.Context())
	user, ok, err := h.database.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid token"})
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	updated, err := h.svc.UpdateEmail(r.Context(), user.ID, body.Email)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrDuplicateEmail):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		default:
			log.Printf("update email error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}

	writeJSON(w, http.StatusOK, meResponse(updated, ""))
}

func (h *AuthHandler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	if err := h.svc.VerifyEmail(r.Context(), body.Token); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
