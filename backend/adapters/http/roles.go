package httpadapter

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"zeropad-backend/adapters/db"
)

type roleResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CanRead   bool   `json:"can_read"`
	CanWrite  bool   `json:"can_write"`
	CanDelete bool   `json:"can_delete"`
	CreatedAt string `json:"created_at"`
}

func toRoleResponse(role db.Role) roleResponse {
	return roleResponse{
		ID:        role.ID,
		Name:      role.Name,
		CanRead:   role.CanRead,
		CanWrite:  role.CanWrite,
		CanDelete: role.CanDelete,
		CreatedAt: role.CreatedAt.Format(rfc3339),
	}
}

type roleBody struct {
	Name      string `json:"name"`
	CanRead   bool   `json:"can_read"`
	CanWrite  bool   `json:"can_write"`
	CanDelete bool   `json:"can_delete"`
}

func (h *APIAccessHandler) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	var body roleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	role, err := h.roles.Create(r.Context(), ownerID, body.Name, body.CanRead, body.CanWrite, body.CanDelete)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrDuplicateRoleName):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "role name already exists"})
		default:
			log.Printf("create role error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusCreated, toRoleResponse(role))
}

func (h *APIAccessHandler) handleListRoles(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	roles, err := h.roles.List(r.Context(), ownerID)
	if err != nil {
		log.Printf("list roles error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]roleResponse, len(roles))
	for i, role := range roles {
		out[i] = toRoleResponse(role)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *APIAccessHandler) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	var body roleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	err := h.roles.Update(r.Context(), r.PathValue("id"), ownerID, body.Name, body.CanRead, body.CanWrite, body.CanDelete)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrRoleNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		case errors.Is(err, db.ErrDuplicateRoleName):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "role name already exists"})
		default:
			log.Printf("update role error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *APIAccessHandler) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := h.resolveOwner(w, r)
	if !ok {
		return
	}
	if err := h.roles.Delete(r.Context(), r.PathValue("id"), ownerID); err != nil {
		switch {
		case errors.Is(err, db.ErrRoleNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		default:
			log.Printf("delete role error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
