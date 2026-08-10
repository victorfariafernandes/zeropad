package httpadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

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
	reserved func(http.HandlerFunc) http.HandlerFunc,
) {
	mux.HandleFunc("/pads/", cors(reserved(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.HandleGet(w, r)
		case http.MethodPut:
			h.HandleSet(w, r)
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
	ExpiresAt  *time.Time         `json:"expires_at,omitempty"`
}

type padMetaResponse struct {
	Slug      string             `json:"slug"`
	Encrypted bool               `json:"encrypted"`
	DeriverId encryption.Deriver `json:"deriver_id"`
	ExpiresAt *time.Time         `json:"expires_at,omitempty"`
}

// expiresAt returns the pad's computed expiry, or nil if UpdatedAt is unset
// (a legacy pad, or an object outside defaultPrefix from before the TTL
// feature) — nil means the client should render no countdown.
func expiresAt(pad store.Pad) *time.Time {
	if pad.UpdatedAt.IsZero() {
		return nil
	}
	t := pad.UpdatedAt.Add(store.DefaultPadTTL)
	return &t
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
				ExpiresAt: expiresAt(pad),
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
		ExpiresAt:  expiresAt(pad),
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

	if status, msg, ok := checkWriteToken(exists, existingPad, body.WriteToken); !ok {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}

	newHashedToken := resolveHashedWriteToken(exists, existingPad, body.Encrypted, body.WriteToken, body.NewWriteToken)

	pad := store.Pad{
		Content:          body.Content,
		Encrypted:        body.Encrypted,
		VerifyBlob:       body.VerifyBlob,
		DeriverId:        body.DeriverId,
		HashedWriteToken: newHashedToken,
		UpdatedAt:        time.Now().UTC(),
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
		ExpiresAt:  expiresAt(pad),
	})
}

// checkWriteToken validates a write request against an existing encrypted
// pad's stored token hash. Returns ok=false with the status/message to write
// if the token is missing or wrong; ok=true if the write may proceed.
func checkWriteToken(exists bool, existingPad store.Pad, providedToken string) (status int, msg string, ok bool) {
	if !exists || !existingPad.Encrypted || existingPad.HashedWriteToken == "" {
		return 0, "", true
	}
	if providedToken == "" {
		return http.StatusForbidden, "write token required", false
	}
	if sha256Hex(providedToken) != existingPad.HashedWriteToken {
		return http.StatusForbidden, "invalid write token", false
	}
	return 0, "", true
}

// resolveHashedWriteToken computes the token hash to store for a write,
// handling first encryption, unchanged re-encryption, and key-change flows.
func resolveHashedWriteToken(exists bool, existingPad store.Pad, encrypted bool, writeToken, newWriteToken string) string {
	switch {
	case !encrypted:
		return ""
	case exists && existingPad.Encrypted && existingPad.HashedWriteToken != "" && newWriteToken != "":
		return sha256Hex(newWriteToken)
	case exists && existingPad.Encrypted && existingPad.HashedWriteToken != "":
		return existingPad.HashedWriteToken
	default:
		return sha256Hex(writeToken)
	}
}

func slugFrom(r *http.Request) string {
	slug := strings.TrimPrefix(r.URL.Path, "/pads/")
	if slug == "" || strings.Contains(slug, "/") {
		return ""
	}
	return slug
}
