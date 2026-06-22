package httpadapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"zeropad-backend/adapters/db"
	authsvc "zeropad-backend/services/auth"
)

// passkeySession is server-side state kept between begin and finish.
type passkeySession struct {
	data      *webauthn.SessionData
	expiresAt time.Time
}

// AuthHandler handles all /auth/* routes.
type AuthHandler struct {
	svc      *authsvc.Service
	passkey  *authsvc.PasskeyService
	cors     func(http.HandlerFunc) http.HandlerFunc
	session  func(http.HandlerFunc) http.HandlerFunc
	database *db.DB

	mu            sync.Mutex
	registrations map[string]passkeySession // key: userID
	loginSessions map[string]passkeySession // key: username
}

func NewAuthHandler(
	svc *authsvc.Service,
	passkey *authsvc.PasskeyService,
	database *db.DB,
	cors func(http.HandlerFunc) http.HandlerFunc,
	session func(http.HandlerFunc) http.HandlerFunc,
) *AuthHandler {
	return &AuthHandler{
		svc:           svc,
		passkey:       passkey,
		database:      database,
		cors:          cors,
		session:       session,
		registrations: make(map[string]passkeySession),
		loginSessions: make(map[string]passkeySession),
	}
}

func (h *AuthHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "POST /auth/signup", h.handleSignup)
	h.handle(mux, "POST /auth/login", h.handleLogin)
	h.handle(mux, "GET /auth/me", h.session(h.handleMe))

	if h.passkey != nil {
		h.handle(mux, "POST /auth/passkey/register/begin", h.session(h.handlePasskeyRegisterBegin))
		h.handle(mux, "POST /auth/passkey/register/finish", h.session(h.handlePasskeyRegisterFinish))
		h.handle(mux, "POST /auth/passkey/login/begin", h.handlePasskeyLoginBegin)
		h.handle(mux, "POST /auth/passkey/login/finish", h.handlePasskeyLoginFinish)
	}
}

func (h *AuthHandler) handle(mux *http.ServeMux, pattern string, fn http.HandlerFunc) {
	mux.HandleFunc(pattern, h.cors(fn))
}

// ─── Signup ──────────────────────────────────────────────────────────────────

func (h *AuthHandler) handleSignup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username      string `json:"username"`
		Email         string `json:"email"`
		Method        string `json:"method"`
		Password      string `json:"password"`
		WalletAddress string `json:"wallet_address"`
		SIWESignature string `json:"siwe_signature"`
		SIWEMessage   string `json:"siwe_message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
		return
	}

	token, err := h.svc.Signup(r.Context(), authsvc.SignupRequest{
		Username:      body.Username,
		Email:         body.Email,
		Password:      body.Password,
		WalletAddress: body.WalletAddress,
		SIWESignature: body.SIWESignature,
		SIWEMessage:   body.SIWEMessage,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrDuplicateUsername):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already taken"})
		case errors.Is(err, db.ErrDuplicateEmail):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		case errors.Is(err, db.ErrDuplicateWallet):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "wallet already registered"})
		case errors.Is(err, authsvc.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		default:
			log.Printf("signup error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

// ─── Login ───────────────────────────────────────────────────────────────────

func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username      string `json:"username"`
		Method        string `json:"method"`
		Password      string `json:"password"`
		WalletAddress string `json:"wallet_address"`
		SIWESignature string `json:"siwe_signature"`
		SIWEMessage   string `json:"siwe_message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	var (
		token string
		err   error
	)
	switch body.Method {
	case "password":
		token, err = h.svc.LoginPassword(r.Context(), body.Username, body.Password)
	case "siwe":
		token, err = h.svc.LoginWallet(r.Context(), body.Username, body.WalletAddress, body.SIWESignature, body.SIWEMessage)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "method must be 'password' or 'siwe'"})
		return
	}

	if err != nil {
		switch {
		case errors.Is(err, authsvc.ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		case errors.Is(err, authsvc.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		default:
			log.Printf("login error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ─── Me ──────────────────────────────────────────────────────────────────────

func (h *AuthHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := authsvc.ClaimsFromContext(r.Context())

	user, ok, err := h.database.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid token"})
		return
	}
	hasPasskey, _ := h.database.HasPasskey(r.Context(), user.ID)

	resp := map[string]any{
		"id":          user.ID,
		"username":    user.Username,
		"has_passkey": hasPasskey,
	}
	if user.Email != "" {
		resp["email"] = user.Email
	}
	if user.WalletAddress != "" {
		resp["wallet_address"] = user.WalletAddress
	}
	writeJSON(w, http.StatusOK, resp)
}

// ─── Passkey registration ────────────────────────────────────────────────────

func (h *AuthHandler) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	claims := authsvc.ClaimsFromContext(r.Context())
	user, ok, err := h.database.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid token"})
		return
	}

	passkeys, err := h.database.GetPasskeysByUserID(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	creation, sessionData, err := h.passkey.BeginRegistration(dbUserAdapter{user: user, passkeys: passkeys})
	if err != nil {
		log.Printf("passkey begin registration: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.mu.Lock()
	h.registrations[user.ID] = passkeySession{data: sessionData, expiresAt: time.Now().Add(2 * time.Minute)}
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, creation)
}

func (h *AuthHandler) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	claims := authsvc.ClaimsFromContext(r.Context())
	user, ok, err := h.database.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid token"})
		return
	}

	h.mu.Lock()
	sess, found := h.registrations[user.ID]
	delete(h.registrations, user.ID)
	h.mu.Unlock()

	if !found || time.Now().After(sess.expiresAt) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no pending registration"})
		return
	}

	passkeys, err := h.database.GetPasskeysByUserID(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	credential, err := h.passkey.FinishRegistration(dbUserAdapter{user: user, passkeys: passkeys}, sess.data, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "verification failed"})
		return
	}

	if err := h.database.CreatePasskey(r.Context(), user.ID, credential.ID, credential.PublicKey); err != nil {
		log.Printf("store passkey: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// ─── Passkey login ───────────────────────────────────────────────────────────

func (h *AuthHandler) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	user, ok, err := h.database.GetUserByUsername(r.Context(), body.Username)
	if err != nil || !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	passkeys, err := h.database.GetPasskeysByUserID(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	assertion, sessionData, err := h.passkey.BeginLogin(dbUserAdapter{user: user, passkeys: passkeys})
	if err != nil {
		log.Printf("passkey begin login: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.mu.Lock()
	h.loginSessions[body.Username] = passkeySession{data: sessionData, expiresAt: time.Now().Add(2 * time.Minute)}
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, assertion)
}

func (h *AuthHandler) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	// Decode body once; FinishLogin needs the credential sub-object as the body.
	raw := make(map[string]json.RawMessage)
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	var username string
	if u, ok := raw["username"]; ok {
		_ = json.Unmarshal(u, &username)
	}

	user, ok, err := h.database.GetUserByUsername(r.Context(), username)
	if err != nil || !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	h.mu.Lock()
	sess, found := h.loginSessions[username]
	delete(h.loginSessions, username)
	h.mu.Unlock()

	if !found || time.Now().After(sess.expiresAt) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no pending login"})
		return
	}

	passkeys, err := h.database.GetPasskeysByUserID(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	credentialRaw, hasCred := raw["credential"]
	if !hasCred {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing credential"})
		return
	}

	// Rebuild request so the webauthn library can read the credential from r.Body.
	credBytes, _ := credentialRaw.MarshalJSON()
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(credBytes))

	credential, err := h.passkey.FinishLogin(dbUserAdapter{user: user, passkeys: passkeys}, sess.data, r2)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "verification failed"})
		return
	}

	if err := h.database.UpdatePasskeySignCount(r.Context(), credential.ID, credential.Authenticator.SignCount); err != nil {
		log.Printf("update sign count: %v", err)
	}

	token, err := authsvc.IssueToken(h.svc.Secret(), user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ─── dbUserAdapter ───────────────────────────────────────────────────────────

// dbUserAdapter adapts db.User + []db.Passkey to authsvc.PasskeyUser.
type dbUserAdapter struct {
	user     db.User
	passkeys []db.Passkey
}

func (a dbUserAdapter) GetID() []byte {
	hex := strings.ReplaceAll(a.user.ID, "-", "")
	b := make([]byte, 16)
	for i := 0; i < 16; i++ {
		b[i] = (hexNibble(hex[i*2]) << 4) | hexNibble(hex[i*2+1])
	}
	return b
}

func (a dbUserAdapter) GetUsername() string { return a.user.Username }

func (a dbUserAdapter) GetCredentials() []authsvc.PasskeyCredential {
	out := make([]authsvc.PasskeyCredential, len(a.passkeys))
	for i, p := range a.passkeys {
		out[i] = authsvc.PasskeyCredential{
			ID:        p.CredentialID,
			PublicKey: p.PublicKey,
			SignCount: p.SignCount,
		}
	}
	return out
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
