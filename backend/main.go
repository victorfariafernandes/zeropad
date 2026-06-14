// Run: go run main.go (requires Go 1.21+)
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	httpadapter "zeropad-backend/adapters/http"
	"zeropad-backend/adapters/store"
	"zeropad-backend/middlewares"
	padsvc "zeropad-backend/services/pad"
)

func selectStore() store.PadStore {
	bucket := os.Getenv("OCI_BUCKET_NAME")
	namespace := os.Getenv("OCI_NAMESPACE")
	if bucket != "" && namespace != "" {
		s, err := store.NewOCIPadStore(namespace, bucket)
		if err != nil {
			log.Fatalf("failed to init OCI store: %v", err)
		}
		log.Printf("using OCI Object Storage bucket=%s namespace=%s", bucket, namespace)
		return s
	}
	log.Printf("OCI_BUCKET_NAME/OCI_NAMESPACE not set — using in-memory store")
	return store.NewMemoryPadStore()
}

func main() {
	padStore := selectStore()
	padHandler := httpadapter.NewPadHandler(padsvc.New(padStore))

	origin := os.Getenv("ALLOW_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000"
	}

	mux := http.NewServeMux()
	cors := middlewares.CORS(origin)
	writeLimiter := middlewares.NewRateLimit(10)

	padHandler.Register(mux, cors, writeLimiter)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			log.Printf("health encode error: %v", err)
		}
	})

	log.Printf("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
