package httpadapter

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"zeropad-backend/adapters/store"
	"zeropad-backend/encryption"
	padsvc "zeropad-backend/services/pad"
)

func newTestHandler() (*PadHandler, *store.MemoryPadStore) {
	memStore := store.NewMemoryPadStore()
	svc := padsvc.New(memStore)
	return NewPadHandler(svc), memStore
}

func putPad(t *testing.T, h *PadHandler, slug string) padResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"content":   "hello",
		"encrypted": false,
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/pads/"+slug, bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleSet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT %q: status = %d, body = %s", slug, w.Code, w.Body.String())
	}
	var resp padResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	return resp
}

func getPad(t *testing.T, h *PadHandler, slug string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/pads/"+slug, nil)
	w := httptest.NewRecorder()
	h.HandleGet(w, req)
	return w
}

func TestHandleSet_FreshWriteReturnsExpiresAtNearTTL(t *testing.T) {
	h, _ := newTestHandler()

	before := time.Now()
	resp := putPad(t, h, "fresh-pad")
	after := time.Now()

	if resp.ExpiresAt == nil {
		t.Fatal("expected non-nil ExpiresAt on a fresh write")
	}
	wantMin := before.Add(store.DefaultPadTTL)
	wantMax := after.Add(store.DefaultPadTTL)
	if resp.ExpiresAt.Before(wantMin) || resp.ExpiresAt.After(wantMax) {
		t.Fatalf("ExpiresAt = %v, want between %v and %v", resp.ExpiresAt, wantMin, wantMax)
	}
}

func TestHandleGet_DoesNotReStampExpiresAt(t *testing.T) {
	h, _ := newTestHandler()

	putResp := putPad(t, h, "stable-pad")

	w := getPad(t, h, "stable-pad")
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", w.Code, w.Body.String())
	}
	var getResp padResponse
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}

	if getResp.ExpiresAt == nil || putResp.ExpiresAt == nil {
		t.Fatal("expected non-nil ExpiresAt on both PUT and GET responses")
	}
	if !getResp.ExpiresAt.Equal(*putResp.ExpiresAt) {
		t.Fatalf("GET must not re-stamp: PUT ExpiresAt = %v, GET ExpiresAt = %v", putResp.ExpiresAt, getResp.ExpiresAt)
	}
}

func TestHandleSet_RefreshesExpiresAtOnEachWrite(t *testing.T) {
	h, _ := newTestHandler()

	first := putPad(t, h, "edited-pad")
	time.Sleep(2 * time.Millisecond) // ensure a distinguishable time.Now() on the second write
	second := putPad(t, h, "edited-pad")

	if first.ExpiresAt == nil || second.ExpiresAt == nil {
		t.Fatal("expected non-nil ExpiresAt on both writes")
	}
	if !second.ExpiresAt.After(*first.ExpiresAt) {
		t.Fatalf("second write's ExpiresAt (%v) should be after the first's (%v)", second.ExpiresAt, first.ExpiresAt)
	}
}

func TestHandleGet_LegacyPadWithoutUpdatedAt_OmitsExpiresAt(t *testing.T) {
	tests := []struct {
		name string
		slug string
		pad  store.Pad
		// header is set on the GET request, if non-empty.
		writeToken string
	}{
		{
			name: "unencrypted legacy pad",
			slug: "legacy-plain",
			pad:  store.Pad{Content: "old content"},
		},
		{
			name: "encrypted legacy pad, no write token supplied",
			slug: "legacy-locked",
			pad: store.Pad{
				Content:          "ciphertext",
				Encrypted:        true,
				DeriverId:        encryption.DeriverPassword,
				HashedWriteToken: "deadbeef",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, memStore := newTestHandler()
			// Seed directly into the store, bypassing HandleSet, to simulate
			// a pad written before the UpdatedAt field existed — its
			// zero-value UpdatedAt must never surface as a fabricated
			// expires_at.
			memStore.Set(tt.slug, tt.pad)

			w := getPad(t, h, tt.slug)
			if w.Code != http.StatusOK {
				t.Fatalf("GET status = %d, body = %s", w.Code, w.Body.String())
			}

			var raw map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Fatalf("decode response as map: %v", err)
			}
			if _, ok := raw["expires_at"]; ok {
				t.Fatalf("expected no expires_at key for a legacy pad with zero UpdatedAt, got: %s", w.Body.String())
			}
		})
	}
}
