package httpadapter

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"zeropad-backend/adapters/db"
	aclsvc "zeropad-backend/services/acl"
	apikeysvc "zeropad-backend/services/apikey"
	authsvc "zeropad-backend/services/auth"
	rolesvc "zeropad-backend/services/role"
)

// APIAccessHandler handles the session-authenticated dashboard routes for
// managing API keys, roles, and ACL grants (/api-keys, /roles, /acl).
type APIAccessHandler struct {
	apiKeys  *apikeysvc.Service
	roles    *rolesvc.Service
	acl      *aclsvc.Service
	database *db.DB
	cors     func(http.HandlerFunc) http.HandlerFunc
	session  func(http.HandlerFunc) http.HandlerFunc
}

func NewAPIAccessHandler(
	apiKeys *apikeysvc.Service,
	roles *rolesvc.Service,
	acl *aclsvc.Service,
	database *db.DB,
	cors func(http.HandlerFunc) http.HandlerFunc,
	session func(http.HandlerFunc) http.HandlerFunc,
) *APIAccessHandler {
	return &APIAccessHandler{apiKeys: apiKeys, roles: roles, acl: acl, database: database, cors: cors, session: session}
}

func (h *APIAccessHandler) Register(mux *http.ServeMux) {
	// CORS preflight: browsers send OPTIONS before any request carrying an
	// Authorization header. Exact + subtree pattern pairs cover both the bare
	// resource path ("/api-keys") and nested ones ("/api-keys/{id}/roles").
	for _, prefix := range []string{"/api-keys", "/roles", "/acl"} {
		noop := h.cors(func(w http.ResponseWriter, r *http.Request) {})
		mux.HandleFunc("OPTIONS "+prefix, noop)
		mux.HandleFunc("OPTIONS "+prefix+"/", noop)
	}

	h.handle(mux, "POST /api-keys", h.session(h.handleCreateAPIKey))
	h.handle(mux, "GET /api-keys", h.session(h.handleListAPIKeys))
	h.handle(mux, "PUT /api-keys/{id}", h.session(h.handleUpdateAPIKey))
	h.handle(mux, "DELETE /api-keys/{id}", h.session(h.handleRevokeAPIKey))
	h.handle(mux, "POST /api-keys/{id}/roles", h.session(h.handleAttachRole))
	h.handle(mux, "GET /api-keys/{id}/roles", h.session(h.handleListAttachedRoles))
	h.handle(mux, "DELETE /api-keys/{id}/roles/{roleId}", h.session(h.handleDetachRole))

	h.handle(mux, "POST /roles", h.session(h.handleCreateRole))
	h.handle(mux, "GET /roles", h.session(h.handleListRoles))
	h.handle(mux, "PUT /roles/{id}", h.session(h.handleUpdateRole))
	h.handle(mux, "DELETE /roles/{id}", h.session(h.handleDeleteRole))

	h.handle(mux, "POST /acl", h.session(h.handleCreateACL))
	h.handle(mux, "GET /acl", h.session(h.handleListACL))
	h.handle(mux, "DELETE /acl/{id}", h.session(h.handleRevokeACL))
}

func (h *APIAccessHandler) handle(mux *http.ServeMux, pattern string, fn http.HandlerFunc) {
	mux.HandleFunc(pattern, h.cors(fn))
}

// resolveOwner resolves the session claims to the owning user's ID, since
// api_keys/roles/acl are all keyed by users.id (see plan: no accounts split yet).
func (h *APIAccessHandler) resolveOwner(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims := authsvc.ClaimsFromContext(r.Context())
	user, ok, err := h.database.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid token"})
		return "", false
	}
	return user.ID, true
}

// ─── API keys ────────────────────────────────────────────────────────────────

type apiKeyResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Restricted bool    `json:"restricted"`
	CreatedAt  string  `json:"created_at"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
	Key        string  `json:"key,omitempty"` // only present once, on creation
}

func toAPIKeyResponse(k db.APIKey) apiKeyResponse {
	resp := apiKeyResponse{
		ID:         k.ID,
		Name:       k.Name,
		Restricted: k.Restricted,
		CreatedAt:  k.CreatedAt.Format(rfc3339),
	}
	if k.RevokedAt != nil {
		s := k.RevokedAt.Format(rfc3339)
		resp.RevokedAt = &s
	}
	return resp
}

const rfc3339 = "2006-01-02T15:04:05Z07:00"

func (h *APIAccessHandler) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	var body struct {
		Name       string `json:"name"`
		Restricted bool   `json:"restricted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	created, err := h.apiKeys.Create(r.Context(), ownerID, body.Name, body.Restricted)
	if err != nil {
		switch {
		case errors.Is(err, apikeysvc.ErrTierRequired):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "paid tier required"})
		default:
			log.Printf("create api key error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}

	resp := toAPIKeyResponse(created.Key)
	resp.Key = created.Raw
	writeJSON(w, http.StatusCreated, resp)
}

func (h *APIAccessHandler) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	keys, err := h.apiKeys.List(r.Context(), ownerID)
	if err != nil {
		log.Printf("list api keys error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		out[i] = toAPIKeyResponse(k)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *APIAccessHandler) handleUpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	var body struct {
		Name       string `json:"name"`
		Restricted bool   `json:"restricted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	if err := h.apiKeys.Update(r.Context(), r.PathValue("id"), ownerID, body.Name, body.Restricted); err != nil {
		switch {
		case errors.Is(err, db.ErrAPIKeyNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found"})
		default:
			log.Printf("update api key error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *APIAccessHandler) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	if err := h.apiKeys.Revoke(r.Context(), r.PathValue("id"), ownerID); err != nil {
		switch {
		case errors.Is(err, db.ErrAPIKeyNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found"})
		default:
			log.Printf("revoke api key error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *APIAccessHandler) handleAttachRole(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	var body struct {
		RoleID string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RoleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role_id is required"})
		return
	}
	if err := h.apiKeys.AttachRole(r.Context(), r.PathValue("id"), body.RoleID, ownerID); err != nil {
		switch {
		case errors.Is(err, db.ErrAPIKeyNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key or role not found"})
		default:
			log.Printf("attach role error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *APIAccessHandler) handleListAttachedRoles(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	roleIDs, err := h.apiKeys.AttachedRoleIDs(r.Context(), r.PathValue("id"), ownerID)
	if err != nil {
		log.Printf("list attached roles error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if roleIDs == nil {
		roleIDs = []string{}
	}
	writeJSON(w, http.StatusOK, roleIDs)
}

func (h *APIAccessHandler) handleDetachRole(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	if err := h.apiKeys.DetachRole(r.Context(), r.PathValue("id"), r.PathValue("roleId"), ownerID); err != nil {
		log.Printf("detach role error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
