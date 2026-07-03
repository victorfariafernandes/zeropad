package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"zeropad-backend/adapters/db"
	"zeropad-backend/adapters/store"
	"zeropad-backend/encryption"
	"zeropad-backend/middlewares"
	aclsvc "zeropad-backend/services/acl"
	apikeysvc "zeropad-backend/services/apikey"
	padsvc "zeropad-backend/services/pad"
)

// APIPadsHandler serves the API-key-authenticated pad CRUD routes
// (roadmap 3.6): POST/GET/DELETE /api/pads/{slug}.
type APIPadsHandler struct {
	pad      *padsvc.Service
	acl      *aclsvc.Service
	database *db.DB
}

func NewAPIPadsHandler(pad *padsvc.Service, acl *aclsvc.Service, database *db.DB) *APIPadsHandler {
	return &APIPadsHandler{pad: pad, acl: acl, database: database}
}

func (h *APIPadsHandler) Register(
	mux *http.ServeMux,
	cors func(http.HandlerFunc) http.HandlerFunc,
	apiKey func(http.HandlerFunc) http.HandlerFunc,
) {
	mux.HandleFunc("/api/pads/", cors(apiKey(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.handleGet(w, r)
		case http.MethodPost:
			h.handleSet(w, r)
		case http.MethodDelete:
			h.handleDelete(w, r)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
}

// apiSlugFrom extracts the slug from /api/pads/{slug}. Unlike the UI pad
// endpoint, this allows nested subpaths (e.g. "team/eng/notes") since
// wildcard ACL grants target path prefixes.
func apiSlugFrom(r *http.Request) string {
	return strings.TrimPrefix(r.URL.Path, "/api/pads/")
}

// countingWriter tracks bytes written to the response, for bandwidth usage
// accounting, without buffering the response.
type countingWriter struct {
	http.ResponseWriter
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.n += int64(n)
	return n, err
}

// countingReader tracks bytes read from the request body, for bandwidth
// usage accounting, without buffering the body.
type countingReader struct {
	io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.n += int64(n)
	return n, err
}

// decodeCountingBody JSON-decodes r.Body into v and returns the number of
// bytes read.
func decodeCountingBody(r *http.Request, v any) (int64, error) {
	cr := &countingReader{Reader: r.Body}
	err := json.NewDecoder(cr).Decode(v)
	return cr.n, err
}

func (h *APIPadsHandler) recordUsage(r *http.Request, ownerID string, bytesIn, bytesOut int64) {
	if err := h.database.RecordUsage(r.Context(), ownerID, bytesIn, bytesOut); err != nil {
		log.Printf("record api usage: %v", err)
	}
}

// checkQuota reports whether ownerID has remaining request and bandwidth
// quota for today, per its tier. Writes the error response itself if not.
func (h *APIPadsHandler) checkQuota(w http.ResponseWriter, r *http.Request, ownerID string) bool {
	user, ok, err := h.database.GetUserByID(r.Context(), ownerID)
	if err != nil || !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return false
	}
	limits := apikeysvc.Limits(user.Tier)

	usage, err := h.database.GetUsageToday(r.Context(), ownerID)
	if err != nil {
		log.Printf("get usage today: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return false
	}
	if usage.RequestCount >= limits.DailyRequestQuota {
		w.Header().Set("X-RateLimit-Remaining", "0")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		return false
	}
	if usage.BytesIn+usage.BytesOut >= limits.DailyBandwidthByte {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error":       "bandwidth_limit",
			"limit_bytes": limits.DailyBandwidthByte,
			"tier":        user.Tier,
		})
		return false
	}
	return true
}

// resolvePadOwner returns the pad's owner per pad_meta, or — if claim is
// true and it has no owner yet — claims it for keyOwnerID.
func (h *APIPadsHandler) resolvePadOwner(r *http.Request, slug, keyOwnerID string, claim bool) (string, error) {
	ownerID, err := h.database.GetPadOwner(r.Context(), slug)
	if err == nil {
		return ownerID, nil
	}
	if !errors.Is(err, db.ErrPadMetaNotFound) {
		return "", err
	}
	if !claim {
		return "", db.ErrPadMetaNotFound
	}
	if err := h.database.ClaimPad(r.Context(), slug, keyOwnerID); err != nil {
		return "", err
	}
	return keyOwnerID, nil
}

func (h *APIPadsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	slug := apiSlugFrom(r)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing slug"})
		return
	}
	claims := middlewares.APIKeyClaimsFromContext(r.Context())
	if !h.checkQuota(w, r, claims.OwnerID) {
		return
	}

	padOwnerID, err := h.resolvePadOwner(r, slug, claims.OwnerID, false)
	if errors.Is(err, db.ErrPadMetaNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pad not found"})
		return
	}
	if err != nil {
		log.Printf("resolve pad owner: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	allowed, err := h.acl.Check(r.Context(), padOwnerID, claims.OwnerID, claims.Restricted, claims.RoleIDs, slug, aclsvc.ActionRead)
	if err != nil {
		log.Printf("acl check: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	pad, err := h.pad.Get(slug)
	if errors.Is(err, padsvc.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pad not found"})
		return
	}
	if err != nil {
		log.Printf("get pad %q: %v", slug, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	cw := &countingWriter{ResponseWriter: w}
	writeJSON(cw, http.StatusOK, padResponse{
		Slug:       slug,
		Content:    pad.Content,
		Encrypted:  pad.Encrypted,
		VerifyBlob: pad.VerifyBlob,
		DeriverId:  pad.DeriverId,
	})
	h.recordUsage(r, claims.OwnerID, 0, cw.n)
}

func (h *APIPadsHandler) handleSet(w http.ResponseWriter, r *http.Request) {
	slug := apiSlugFrom(r)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing slug"})
		return
	}
	claims := middlewares.APIKeyClaimsFromContext(r.Context())
	if !h.checkQuota(w, r, claims.OwnerID) {
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
	bytesIn, err := decodeCountingBody(r, &body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	// Claim-on-first-write: an unowned slug becomes owned by this key's
	// account the first time it's written through the API.
	padOwnerID, err := h.resolvePadOwner(r, slug, claims.OwnerID, true)
	if err != nil {
		log.Printf("resolve pad owner: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	allowed, err := h.acl.Check(r.Context(), padOwnerID, claims.OwnerID, claims.Restricted, claims.RoleIDs, slug, aclsvc.ActionWrite)
	if err != nil {
		log.Printf("acl check: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	existingPad, svcErr := h.pad.Get(slug)
	exists := !errors.Is(svcErr, padsvc.ErrNotFound)
	if svcErr != nil && !errors.Is(svcErr, padsvc.ErrNotFound) {
		log.Printf("fetch existing %q: %v", slug, svcErr)
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
	}
	if err := h.pad.Set(slug, pad); err != nil {
		log.Printf("set pad %q: %v", slug, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	cw := &countingWriter{ResponseWriter: w}
	writeJSON(cw, http.StatusOK, padResponse{
		Slug:       slug,
		Content:    pad.Content,
		Encrypted:  pad.Encrypted,
		VerifyBlob: pad.VerifyBlob,
		DeriverId:  pad.DeriverId,
	})
	h.recordUsage(r, claims.OwnerID, bytesIn, cw.n)
}

func (h *APIPadsHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	slug := apiSlugFrom(r)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing slug"})
		return
	}
	claims := middlewares.APIKeyClaimsFromContext(r.Context())
	if !h.checkQuota(w, r, claims.OwnerID) {
		return
	}

	padOwnerID, err := h.resolvePadOwner(r, slug, claims.OwnerID, false)
	if errors.Is(err, db.ErrPadMetaNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pad not found"})
		return
	}
	if err != nil {
		log.Printf("resolve pad owner: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	allowed, err := h.acl.Check(r.Context(), padOwnerID, claims.OwnerID, claims.Restricted, claims.RoleIDs, slug, aclsvc.ActionDelete)
	if err != nil {
		log.Printf("acl check: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.pad.Delete(slug); err != nil {
		log.Printf("delete pad %q: %v", slug, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	cw := &countingWriter{ResponseWriter: w}
	writeJSON(cw, http.StatusOK, map[string]bool{"ok": true})
	h.recordUsage(r, claims.OwnerID, 0, cw.n)
}
