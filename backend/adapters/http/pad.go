package httpadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"zeropad-backend/adapters/store"
	"zeropad-backend/encryption"
	padsvc "zeropad-backend/services/pad"
)

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

type PadHandler struct {
	svc *padsvc.Service
}

func NewPadHandler(svc *padsvc.Service) *PadHandler {
	return &PadHandler{svc: svc}
}

func (h *PadHandler) Register(
	mux *http.ServeMux,
	cors func(http.HandlerFunc) http.HandlerFunc,
	rateLimit func(http.HandlerFunc) http.HandlerFunc,
	reserved func(http.HandlerFunc) http.HandlerFunc,
) {
	mux.HandleFunc("/pads/", cors(reserved(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.HandleGet(w, r)
		case http.MethodPut:
			rateLimit(h.HandleSet)(w, r)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
}

type padResponse struct {
	Slug       string             `json:"slug"`
	Content    string             `json:"content"`
	Encrypted  bool               `json:"encrypted"`
	VerifyBlob string             `json:"verify_blob"`
	DeriverId  encryption.Deriver `json:"deriver_id"`
}

type padMetaResponse struct {
	Slug      string             `json:"slug"`
	Encrypted bool               `json:"encrypted"`
	DeriverId encryption.Deriver `json:"deriver_id"`
}

func (h *PadHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	slug := slugFrom(r)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing slug"})
		return
	}
	pad, err := h.svc.Get(slug)
	if errors.Is(err, padsvc.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pad not found"})
		return
	}

	// Encrypted pads with a stored token require X-Write-Token.
	// Legacy encrypted pads (HashedWriteToken == "") are returned in full for
	// backward compatibility; the first PUT will lock them going forward.
	if pad.Encrypted && pad.HashedWriteToken != "" {
		token := r.Header.Get("X-Write-Token")
		if token == "" {
			writeJSON(w, http.StatusOK, padMetaResponse{
				Slug:      slug,
				Encrypted: pad.Encrypted,
				DeriverId: pad.DeriverId,
			})
			return
		}
		if sha256Hex(token) != pad.HashedWriteToken {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid write token"})
			return
		}
	}

	writeJSON(w, http.StatusOK, padResponse{
		Slug:       slug,
		Content:    pad.Content,
		Encrypted:  pad.Encrypted,
		VerifyBlob: pad.VerifyBlob,
		DeriverId:  pad.DeriverId,
	})
}

func (h *PadHandler) HandleSet(w http.ResponseWriter, r *http.Request) {
	slug := slugFrom(r)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing slug"})
		return
	}
	var body struct {
		Content       string             `json:"content"`
		Encrypted     bool               `json:"encrypted"`
		VerifyBlob    string             `json:"verify_blob"`
		DeriverId     encryption.Deriver `json:"deriver_id"`
		WriteToken    string             `json:"write_token"`
		NewWriteToken string             `json:"new_write_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	existingPad, svcErr := h.svc.Get(slug)
	exists := !errors.Is(svcErr, padsvc.ErrNotFound)
	if svcErr != nil && !errors.Is(svcErr, padsvc.ErrNotFound) {
		log.Printf("HandleSet fetch existing %q: %v", slug, svcErr)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if exists && existingPad.Encrypted && existingPad.HashedWriteToken != "" {
		if body.WriteToken == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "write token required"})
			return
		}
		if sha256Hex(body.WriteToken) != existingPad.HashedWriteToken {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid write token"})
			return
		}
	}

	var newHashedToken string
	switch {
	case body.Encrypted && exists && existingPad.Encrypted && existingPad.HashedWriteToken != "" && body.NewWriteToken != "":
		newHashedToken = sha256Hex(body.NewWriteToken)
	case body.Encrypted && exists && existingPad.Encrypted && existingPad.HashedWriteToken != "":
		newHashedToken = existingPad.HashedWriteToken
	case body.Encrypted:
		newHashedToken = sha256Hex(body.WriteToken)
	default:
		newHashedToken = ""
	}

	pad := store.Pad{
		Content:          body.Content,
		Encrypted:        body.Encrypted,
		VerifyBlob:       body.VerifyBlob,
		DeriverId:        body.DeriverId,
		HashedWriteToken: newHashedToken,
	}
	if err := h.svc.Set(slug, pad); err != nil {
		log.Printf("Set pad %q: %v", slug, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, padResponse{
		Slug:       slug,
		Content:    pad.Content,
		Encrypted:  pad.Encrypted,
		VerifyBlob: pad.VerifyBlob,
		DeriverId:  pad.DeriverId,
	})
}

func slugFrom(r *http.Request) string {
	slug := strings.TrimPrefix(r.URL.Path, "/pads/")
	if slug == "" || strings.Contains(slug, "/") {
		return ""
	}
	return slug
}
