// Run: go run main.go (requires Go 1.21+)
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"zeropad-backend/adapters/db"
	httpadapter "zeropad-backend/adapters/http"
	"zeropad-backend/adapters/store"
	"zeropad-backend/middlewares"
	aclsvc "zeropad-backend/services/acl"
	apikeysvc "zeropad-backend/services/apikey"
	authsvc "zeropad-backend/services/auth"
	"zeropad-backend/services/email"
	padsvc "zeropad-backend/services/pad"
	rolesvc "zeropad-backend/services/role"
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
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		log.Fatal("JWT_SECRET env var is required")
	}

	var database *db.DB
	if os.Getenv("POSTGRES_URL") != "" {
		var err error
		database, err = db.Init(context.Background())
		if err != nil {
			log.Fatalf("failed to init database: %v", err)
		}
		defer database.Close()
	}

	padStore := selectStore()
	padService := padsvc.New(padStore)
	padHandler := httpadapter.NewPadHandler(padService)

	origin := os.Getenv("ALLOW_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000"
	}

	mux := http.NewServeMux()
	cors := middlewares.CORS(origin)
	session := middlewares.Session(jwtSecret)

	padHandler.Register(mux, cors, middlewares.Reserved)

	if database != nil {
		resendAPIKey := os.Getenv("RESEND_API_KEY")
		resendFrom := os.Getenv("RESEND_FROM_EMAIL")
		resendTemplateID := os.Getenv("RESEND_TEMPLATE_ID")
		if resendAPIKey == "" || resendFrom == "" || resendTemplateID == "" {
			log.Fatal("RESEND_API_KEY, RESEND_FROM_EMAIL, and RESEND_TEMPLATE_ID env vars are required")
		}
		mailer := email.NewResendSender(resendAPIKey, resendFrom, resendTemplateID)

		frontendURL := os.Getenv("FRONTEND_URL")
		if frontendURL == "" {
			frontendURL = origin
		}

		svc := authsvc.NewService(database, jwtSecret, mailer, frontendURL)

		var passkey *authsvc.PasskeyService
		rpID := os.Getenv("WEBAUTHN_RP_ID")
		rpOrigin := os.Getenv("WEBAUTHN_RP_ORIGIN")
		rpName := os.Getenv("WEBAUTHN_RP_NAME")
		if rpID != "" && rpOrigin != "" {
			if rpName == "" {
				rpName = "zeropad"
			}
			var err error
			passkey, err = authsvc.NewPasskeyService(rpID, rpOrigin, rpName)
			if err != nil {
				log.Fatalf("failed to init passkey service: %v", err)
			}
			log.Printf("passkey service enabled rpID=%s", rpID)
		}

		authHandler := httpadapter.NewAuthHandler(svc, passkey, database, cors, session)
		authHandler.Register(mux)

		apiKeys := apikeysvc.New(database)
		roles := rolesvc.New(database)
		acl := aclsvc.New(database)

		accessHandler := httpadapter.NewAPIAccessHandler(apiKeys, roles, acl, database, cors, session)
		accessHandler.Register(mux)

		apiPadsHandler := httpadapter.NewAPIPadsHandler(padService, acl, database)
		apiPadsHandler.Register(mux, cors, middlewares.APIKey(apiKeys))
	}

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
