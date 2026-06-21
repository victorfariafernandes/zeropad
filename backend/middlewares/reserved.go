package middlewares

import (
	"net/http"
	"strings"
)

func Reserved(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimPrefix(r.URL.Path, "/pads/")
		if strings.HasPrefix(slug, "_") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"reserved slug"}`))
			return
		}
		next(w, r)
	}
}
