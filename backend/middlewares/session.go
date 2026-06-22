package middlewares

import (
	"net/http"
	"strings"

	authsvc "zeropad-backend/services/auth"
)

// Session validates the JWT in Authorization: Bearer <token> and injects the
// claims into the request context. Returns 401 if the header is missing or invalid.
func Session(secret []byte) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"missing or invalid token"}`))
				return
			}
			raw := strings.TrimPrefix(header, "Bearer ")
			claims, err := authsvc.VerifyToken(secret, raw)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"missing or invalid token"}`))
				return
			}
			next(w, r.WithContext(authsvc.ContextWithClaims(r.Context(), claims)))
		}
	}
}
