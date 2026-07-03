package httpadapter

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"zeropad-backend/adapters/db"
	aclsvc "zeropad-backend/services/acl"
)

type aclResponse struct {
	ID          string `json:"id"`
	SlugPattern string `json:"slug_pattern"`
	RoleID      string `json:"role_id"`
	CreatedAt   string `json:"created_at"`
}

func toACLResponse(a db.ACL) aclResponse {
	return aclResponse{
		ID:          a.ID,
		SlugPattern: a.SlugPattern,
		RoleID:      a.RoleID,
		CreatedAt:   a.CreatedAt.Format(rfc3339),
	}
}

func (h *APIAccessHandler) handleCreateACL(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	var body struct {
		SlugPattern string `json:"slug_pattern"`
		RoleID      string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.SlugPattern == "" || body.RoleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug_pattern and role_id are required"})
		return
	}

	grant, err := h.acl.Grant(r.Context(), ownerID, body.SlugPattern, body.RoleID)
	if err != nil {
		switch {
		case errors.Is(err, aclsvc.ErrInvalidSlugPattern):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			log.Printf("create acl error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusCreated, toACLResponse(grant))
}

func (h *APIAccessHandler) handleListACL(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	grants, err := h.acl.List(r.Context(), ownerID)
	if err != nil {
		log.Printf("list acl error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]aclResponse, len(grants))
	for i, g := range grants {
		out[i] = toACLResponse(g)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *APIAccessHandler) handleRevokeACL(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	if err := h.acl.Revoke(r.Context(), r.PathValue("id"), ownerID); err != nil {
		switch {
		case errors.Is(err, db.ErrACLNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "acl grant not found"})
		default:
			log.Printf("revoke acl error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
