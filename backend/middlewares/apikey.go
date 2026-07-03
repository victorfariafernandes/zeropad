package middlewares

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"zeropad-backend/adapters/db"
	apikeysvc "zeropad-backend/services/apikey"
)

type contextKey string

const apiKeyClaimsKey contextKey = "apiKeyClaims"

// APIKeyClaims is the resolved identity of an authenticated API key,
// injected into the request context by APIKey.
type APIKeyClaims struct {
	OwnerID    string
	Restricted bool
	RoleIDs    []string
}

func APIKeyClaimsFromContext(ctx context.Context) APIKeyClaims {
	c, _ := ctx.Value(apiKeyClaimsKey).(APIKeyClaims)
	return c
}

func contextWithAPIKeyClaims(ctx context.Context, c APIKeyClaims) context.Context {
	return context.WithValue(ctx, apiKeyClaimsKey, c)
}

// APIKey validates the raw key in Authorization: Bearer <key> against the
// api_keys table and injects APIKeyClaims into the request context.
// Distinct from Session (JWT) — used only on the /api/pads/* routes.
func APIKey(svc *apikeysvc.Service) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"missing or invalid api key"}`))
				return
			}
			raw := strings.TrimPrefix(header, "Bearer ")
			key, roleIDs, err := svc.Authenticate(r.Context(), raw)
			if errors.Is(err, db.ErrAPIKeyNotFound) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"missing or invalid api key"}`))
				return
			}
			if err != nil {
				log.Printf("api key authenticate: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal error"}`))
				return
			}
			claims := APIKeyClaims{OwnerID: key.OwnerID, Restricted: key.Restricted, RoleIDs: roleIDs}
			next(w, r.WithContext(contextWithAPIKeyClaims(r.Context(), claims)))
		}
	}
}
