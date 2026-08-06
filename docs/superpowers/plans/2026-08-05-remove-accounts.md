# Remove Accounts/Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove every account-dependent feature (login/signup, sessions/JWT, passkeys, profile settings, email verification, API keys, roles, ACL) from the backend, frontend, docs, deploy pipeline, and infra, per `docs/superpowers/specs/2026-08-05-remove-accounts-design.md`.

**Architecture:** Delete the account subsystem wholesale rather than disabling it — no feature flags, no dead code paths. The backend loses its Postgres dependency entirely (pad storage is in-memory/OCI and never used Postgres). The frontend loses everything under `app/system/` plus the two `_lib` files and one component that talk to it. Infra loses the CloudNativePG Postgres cluster that existed only to back accounts.

**Tech Stack:** Go 1.25 (backend), Next.js 16 / TypeScript (frontend), Terraform (OCI provider), Ansible (k3s deploy), GitHub Actions.

## Global Constraints

- `http.StatusXxx` constants only — no raw integers in Go (existing code already follows this; nothing new to write here since this plan only deletes/simplifies).
- All Go JSON responses go through `writeJSON(w, statusCode, payload)` — defined in `backend/adapters/http/pad.go`, unaffected by this plan.
- `@/` path alias for all internal TypeScript imports — no `../../` paths.
- Append an entry to `CHANGELOG.md` before ending the session (Task 10 does this).
- Do not run `terraform apply` or `ansible-playbook ... --tags deploy` (without `--syntax-check`) as part of this work — IaC files are edited and validated locally only. Applying is a separate, real-infrastructure-affecting action left for the user.

---

### Task 1: Backend — remove the account subsystem and simplify `main.go`

**Files:**
- Delete: `backend/services/auth/` (jwt.go, passkey.go, service.go)
- Delete: `backend/services/apikey/` (service.go, limits.go)
- Delete: `backend/services/role/`
- Delete: `backend/services/acl/` (service.go, service_test.go)
- Delete: `backend/services/email/` (sender.go, resend.go)
- Delete: `backend/adapters/http/auth.go`, `profile.go`, `apikeys.go`, `roles.go`, `acl.go`, `api_pads.go`
- Delete: `backend/adapters/db/user.go`, `acl.go`, `apikey.go`, `padmeta.go`, `role.go`, `usage.go`, `db.go`, `db_test.go`
- Delete: `backend/middlewares/session.go`, `backend/middlewares/apikey.go`
- Modify: `backend/main.go`
- Modify: `backend/go.mod`, `backend/go.sum`

**Interfaces:**
- Consumes: `padsvc.New(store.PadStore) *pad.Service` and `httpadapter.NewPadHandler(*pad.Service) *PadHandler` (both pre-existing, unaffected)
- Produces: nothing new — this task only removes code. Task 2 depends on `backend/adapters/db/migrations/` still existing as a directory of plain SQL files (no Go code reads them anymore after this task).

- [ ] **Step 1: Delete the account services**

```bash
cd backend
git rm -r services/auth services/apikey services/role services/acl services/email
```

- [ ] **Step 2: Delete the account HTTP adapters**

```bash
git rm adapters/http/auth.go adapters/http/profile.go adapters/http/apikeys.go \
       adapters/http/roles.go adapters/http/acl.go adapters/http/api_pads.go
```

- [ ] **Step 3: Delete the account DB adapters (keep the `migrations/` directory)**

```bash
git rm adapters/db/user.go adapters/db/acl.go adapters/db/apikey.go \
       adapters/db/padmeta.go adapters/db/role.go adapters/db/usage.go \
       adapters/db/db.go adapters/db/db_test.go
```

Confirm `adapters/db/migrations/` still exists and nothing else remains in `adapters/db/`:

```bash
ls adapters/db
```

Expected: only `migrations/`.

- [ ] **Step 4: Delete the account middlewares**

```bash
git rm middlewares/session.go middlewares/apikey.go
```

- [ ] **Step 5: Rewrite `main.go`**

Replace the full contents of `backend/main.go` with:

```go
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
	padService := padsvc.New(padStore)
	padHandler := httpadapter.NewPadHandler(padService)

	origin := os.Getenv("ALLOW_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000"
	}

	mux := http.NewServeMux()
	cors := middlewares.CORS(origin)

	padHandler.Register(mux, cors, middlewares.Reserved)

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
```

- [ ] **Step 6: Remove now-unused dependencies from `go.mod` and tidy**

```bash
go mod tidy
```

This should remove `github.com/ethereum/go-ethereum`, `github.com/go-webauthn/webauthn`, `github.com/go-webauthn/x`, `github.com/golang-jwt/jwt/v5`, `github.com/jackc/pgx/v5` (and its transitive `pgpassfile`/`pgservicefile`/`puddle` deps), and `github.com/resend/resend-go/v3` from `go.mod`/`go.sum`. `golang.org/x/crypto` and the OCI SDK stay (used by `adapters/store` for OCI object storage / by `encryption`).

Verify by checking the diff:

```bash
git diff go.mod
```

Expected: only removals, no new additions.

- [ ] **Step 7: Build, vet, and test**

```bash
go build ./...
go vet ./...
go test ./...
```

Expected: all three succeed. `go test ./...` should show `services/pad` and `adapters/store` passing (their tests are untouched) and no other packages listed (everything else with tests was deleted in Steps 1–3).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: remove account subsystem from backend"
```

---

### Task 2: Backend — add the account-tables teardown migration

**Files:**
- Create: `backend/adapters/db/migrations/009_drop_accounts.sql`

**Interfaces:**
- Consumes: nothing (plain SQL, no Go code)
- Produces: nothing consumed by later tasks — this file is not wired into any Go code (Task 1 deleted `db.go`, the only place that ever executed migration files). It exists as the last entry in the numbered-migration sequence for anyone who still has a Postgres instance running (local dev via `scripts/dev-postgres.sh`, or the production cluster before it's decommissioned in Task 6) and wants to run it manually: `psql "$POSTGRES_URL" -f backend/adapters/db/migrations/009_drop_accounts.sql`.

- [ ] **Step 1: Write the migration**

```sql
DROP TABLE IF EXISTS api_key_roles CASCADE;
DROP TABLE IF EXISTS acl CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS api_usage CASCADE;
DROP TABLE IF EXISTS pad_meta CASCADE;
DROP TABLE IF EXISTS email_verification_tokens CASCADE;
DROP TABLE IF EXISTS passkeys CASCADE;
DROP TABLE IF EXISTS users CASCADE;
```

- [ ] **Step 2: Verify it runs cleanly against a local Postgres**

```bash
eval "$(./scripts/dev-postgres.sh | tail -1)"
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/001_users.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/002_auth.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/003_email_verification.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/004_pad_meta.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/005_api_keys.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/006_roles_acl.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/007_api_usage.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/008_users_tier.sql
psql "$POSTGRES_URL" -f backend/adapters/db/migrations/009_drop_accounts.sql
psql "$POSTGRES_URL" -c "\dt"
```

Expected: the final `\dt` shows no tables (or only tables unrelated to accounts, if any existed — there are none in this schema).

- [ ] **Step 3: Commit**

```bash
git add backend/adapters/db/migrations/009_drop_accounts.sql
git commit -m "feat: add teardown migration for account tables"
```

---

### Task 3: Frontend — remove account pages, libs, and component

**Files:**
- Delete: `frontend/app/system/` in full (`login/AuthPageClient.tsx`, `login/page.tsx`, `profile/ApiKeysPanel.tsx`, `profile/ProfilePageClient.tsx`, `profile/ProfileSettingsPanel.tsx`, `profile/page.tsx`, `verify-email/VerifyEmailClient.tsx`, `verify-email/page.tsx`)
- Delete: `frontend/app/_lib/auth.ts`, `frontend/app/_lib/apiKeys.ts`
- Delete: `frontend/app/components/UserBubble.tsx`
- Modify: `frontend/app/page.tsx`
- Modify: `frontend/app/[slug]/PadEditor.tsx`

**Interfaces:**
- Consumes: nothing new
- Produces: nothing new — `page.tsx` and `PadEditor.tsx` header layouts lose their right-aligned account control with no replacement

- [ ] **Step 1: Delete the account pages, libs, and component**

```bash
cd frontend
git rm -r app/system
git rm app/_lib/auth.ts app/_lib/apiKeys.ts
git rm app/components/UserBubble.tsx
```

- [ ] **Step 2: Update `app/page.tsx`**

Remove the `UserBubble` import and its usage, and the now-empty header:

```tsx
"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

export default function Home() {
  const router = useRouter();
  const [slug, setSlug] = useState("");

  function go() {
    const clean = slug.trim().replace(/^\/+/, "");
    if (!clean) return;
    if (clean.startsWith("_")) return;
    router.push(`/${clean}`);
  }

  return (
    <div className="flex flex-col flex-1">
      <div className="flex flex-col flex-1 items-center justify-center p-8">
        <main className="flex flex-col items-center gap-6 w-full max-w-md">
        <h1 className="text-4xl font-semibold tracking-tight">zeropad</h1>
        <p className="text-sm text-zinc-500">any url is a pad</p>
        <div className="flex gap-2 w-full">
          <input
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && go()}
            placeholder="page-name"
            className="flex-1 h-10 px-3 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 font-mono text-sm outline-none focus:ring-2 focus:ring-black/20 dark:focus:ring-white/20"
          />
          <button
            onClick={go}
            className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium"
          >
            Go
          </button>
        </div>
        </main>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Update `app/[slug]/PadEditor.tsx`**

Remove the `import { UserBubble } from "@/app/components/UserBubble";` line (line 5).

At the locked-pad header (around line 306), change:

```tsx
          <div className="flex items-center gap-3">
            <span className="font-mono text-sm text-zinc-500">/{slug}</span>
            <UserBubble />
          </div>
```

to:

```tsx
          <div className="flex items-center gap-3">
            <span className="font-mono text-sm text-zinc-500">/{slug}</span>
          </div>
```

At the unlocked-pad header (around line 454), remove the standalone `<UserBubble />` line — delete it, leaving whatever sibling elements precede it (the "Change key"/"Encrypt" button) as the last item in that flex container.

- [ ] **Step 4: Type-check and lint**

```bash
npx tsc --noEmit
npx eslint
```

Expected: both succeed with no errors, and no remaining references to `UserBubble`, `@/app/_lib/auth`, or `@/app/_lib/apiKeys`. Confirm with:

```bash
grep -rn "UserBubble\|_lib/auth\|_lib/apiKeys" app/
```

Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: remove account pages, libs, and UserBubble from frontend"
```

---

### Task 4: Frontend — remove account-dependent Cypress spec and test-only Postgres wiring

**Files:**
- Delete: `frontend/cypress/e2e/api-access.cy.ts`
- Modify: `frontend/cypress.config.ts`
- Modify: `frontend/package.json` (remove `pg` and `@types/pg`)

**Interfaces:**
- Consumes: nothing
- Produces: nothing

- [ ] **Step 1: Delete the API-access spec**

```bash
cd frontend
git rm cypress/e2e/api-access.cy.ts
```

- [ ] **Step 2: Simplify `cypress.config.ts`**

Replace its full contents with:

```ts
import { defineConfig } from "cypress";

export default defineConfig({
  e2e: {
    baseUrl: "http://localhost:3000",
    supportFile: false,
    env: {
      apiUrl: process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080",
    },
  },
});
```

- [ ] **Step 3: Remove the `pg` dependency**

Remove the `"@types/pg": "^8.20.0",` line from `devDependencies` and the `"pg": "^8.22.0",` line from `dependencies` in `package.json`, then:

```bash
pnpm install
```

Expected: `pnpm-lock.yaml` updates to drop `pg` and `@types/pg` and their transitive deps.

- [ ] **Step 4: Type-check**

```bash
npx tsc --noEmit
```

Expected: succeeds — confirms `cypress.config.ts` has no dangling reference to the `pg` import.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: remove account-dependent Cypress spec and pg dependency"
```

---

### Task 5: Docs — remove account sections from features, architecture, and API spec

**Files:**
- Modify: `docs/features.md`
- Modify: `docs/architecture.md`
- Modify: `docs/api-spec.md`

**Interfaces:** none (documentation only)

- [ ] **Step 1: Update `docs/features.md`**

Remove the "Profile Settings" section (lines 33–39, from `### Profile Settings` through the line before the next `---`).

- [ ] **Step 2: Update `docs/architecture.md`**

In the backend directory-structure block, remove the `auth/`, `apikey/`, `role/`, `acl/`, and `email/` entries under `services/`, and the entire `db/` entry under `adapters/` (replace its multi-line comment with nothing — `adapters/` now only lists `http/` and `store/`). Remove the `session.go` and `apikey.go` lines under `middlewares/`.

Remove the "### Email sending" section in full.

Remove the entire "## API Access: Keys, Roles, ACL" section (from that heading through the line before "## Key Derivation").

In the "## Environment Variables" table, remove any row that isn't `NEXT_PUBLIC_API_URL`, and add rows for what `main.go` (post-Task-1) actually reads: `ALLOW_ORIGIN`, `OCI_BUCKET_NAME`, `OCI_NAMESPACE` (all backend, all optional with the defaults/behavior described in Task 1's `main.go`).

- [ ] **Step 3: Update `docs/api-spec.md`**

Remove the entire "## Authentication" section (`GET /auth/me` through `POST /auth/verify-email`).

Remove the entire "## API Access (Keys, Roles, ACL)" section (`POST /api-keys` through `POST/GET/DELETE /api/pads/{slug}`).

- [ ] **Step 4: Verify no dangling cross-references**

```bash
grep -rn "auth/me\|api-keys\|/roles\|/acl\|api/pads\|Profile Settings\|API Access" docs/
```

Expected: no output (or only incidental matches you've confirmed are unrelated, e.g. a heading like "## Endpoints" that happens to contain none of these strings — re-check any hit by hand).

- [ ] **Step 5: Commit**

```bash
git add docs/features.md docs/architecture.md docs/api-spec.md
git commit -m "docs: remove account/API-access sections"
```

---

### Task 6: Infra — remove the Postgres Kubernetes manifests and backend env wiring

**Files:**
- Delete: `infra/k8s/postgres/` (`namespace.yaml`, `cluster.yaml`, `scheduled-backup.yaml`)
- Modify: `infra/k8s/backend/deployment.yaml`

**Interfaces:** none (Kubernetes manifests)

- [ ] **Step 1: Delete the postgres manifests**

```bash
git rm -r infra/k8s/postgres
```

- [ ] **Step 2: Update `infra/k8s/backend/deployment.yaml`**

Remove the `env:` block's `POSTGRES_URL` and `JWT_SECRET` and `RESEND_API_KEY` entries entirely, leaving just `envFrom`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: zeropad
spec:
  replicas: 2
  selector:
    matchLabels:
      app: backend
  template:
    metadata:
      labels:
        app: backend
    spec:
      containers:
        - name: backend
          image: ghcr.io/victorfariafernandes/zeropad-backend:latest
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef:
                name: backend-env
          resources:
            requests:
              memory: "128Mi"
              cpu: "50m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 15
            periodSeconds: 20
            failureThreshold: 3
```

- [ ] **Step 3: Verify YAML is well-formed**

```bash
python3 -c "import yaml,sys; [print(d) for d in yaml.safe_load_all(open('infra/k8s/backend/deployment.yaml'))]"
```

Expected: prints the parsed manifest with no error.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: remove postgres and account env wiring from k8s manifests"
```

---

### Task 7: Infra — remove Postgres provisioning from the Ansible deploy role

**Files:**
- Modify: `infra/ansible/roles/k8s-manifests/tasks/main.yml`

**Interfaces:** none

- [ ] **Step 1: Remove the CloudNativePG operator install/wait tasks**

Delete these tasks: "Add CloudNativePG Helm repo", "Install or upgrade CloudNativePG operator", "Wait for CNPG operator to be ready".

- [ ] **Step 2: Remove the Postgres cluster tasks**

Delete these tasks: "Apply postgres namespace", "Create postgres backup credentials secret", "Apply PostgreSQL cluster", "Apply scheduled backup", "Wait for PostgreSQL cluster to be ready", "Copy CNPG app secret to zeropad namespace".

- [ ] **Step 3: Remove the backend secrets task and trim the ConfigMap task**

Delete the "Create or update backend secrets" task entirely (it only ever set `JWT_SECRET` and `RESEND_API_KEY`, both gone).

In "Create or update backend ConfigMap", remove the `--from-literal=RESEND_FROM_EMAIL={{ resend_from_email }}` and `--from-literal=RESEND_TEMPLATE_ID={{ resend_template_id }}` lines, leaving:

```yaml
- name: Create or update backend ConfigMap
  shell: >
    /usr/local/bin/k3s kubectl create configmap backend-env
    --from-literal=ALLOW_ORIGIN=https://{{ domain }}
    --from-literal=OCI_BUCKET_NAME=zeropad-pads
    --from-literal=OCI_NAMESPACE={{ oci_namespace }}
    --namespace=zeropad
    --dry-run=client -o yaml | /usr/local/bin/k3s kubectl apply -f -
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml
```

The resulting file should read, in order: "Ensure rsync is installed", "Validate required deploy variables", "Synchronize k8s manifests to server", "Apply namespace", "Install Helm if not present", "Add ingress-nginx Helm repo", "Update Helm repos", "Install or upgrade ingress-nginx", "Wait for ingress-nginx controller to be ready", "Create or update backend ConfigMap" (trimmed above), "Create or update frontend release-tag ConfigMap", "Apply backend deployment", "Apply backend service", "Patch backend image tag", "Apply frontend manifests", "Restart frontend deployment to fetch new release", "Apply ingress manifest", "Wait for backend rollout", "Wait for frontend rollout".

- [ ] **Step 4: Syntax-check the playbook**

```bash
cd infra/ansible
ansible-playbook playbook.yml -i inventory.ini --syntax-check
```

Expected: `playbook: playbook.yml` with no errors. (This does not require real inventory values or connect to any host.)

- [ ] **Step 5: Commit**

```bash
git add infra/ansible/roles/k8s-manifests/tasks/main.yml
git commit -m "feat: remove postgres provisioning from ansible deploy role"
```

---

### Task 8: Infra — remove the Postgres backup bucket and IAM policy from Terraform

**Files:**
- Modify: `infra/terraform/main.tf`
- Modify: `infra/terraform/modules/storage/main.tf`
- Modify: `infra/terraform/modules/storage/outputs.tf`

**Interfaces:** none

- [ ] **Step 1: Remove the IAM policy in `infra/terraform/main.tf`**

Delete the entire `resource "oci_identity_policy" "postgres_backup_object_storage"` block (lines 110–117).

- [ ] **Step 2: Remove the bucket in `infra/terraform/modules/storage/main.tf`**

Delete the entire `resource "oci_objectstorage_bucket" "postgres_backups"` block.

- [ ] **Step 3: Remove the output in `infra/terraform/modules/storage/outputs.tf`**

Delete the entire `output "postgres_backups_bucket_name"` block.

- [ ] **Step 4: Validate**

```bash
cd infra/terraform
terraform validate
```

Expected: `Success! The configuration is valid.` This checks syntax and internal references only — it does not touch the real `terraform.tfstate` or provision/destroy anything. Do **not** run `terraform plan` or `terraform apply` as part of this task.

- [ ] **Step 5: Commit**

```bash
git add infra/terraform/main.tf infra/terraform/modules/storage/main.tf infra/terraform/modules/storage/outputs.tf
git commit -m "feat: remove postgres backup bucket and IAM policy from terraform"
```

---

### Task 9: CI — remove account secrets from the deploy workflow

**Files:**
- Modify: `.github/workflows/deploy.yml`

**Interfaces:** none

- [ ] **Step 1: Trim the `deploy` job's env block**

In the "Deploy to k3s cluster" step, remove the `POSTGRES_BACKUP_ACCESS_KEY_ID`, `POSTGRES_BACKUP_SECRET_ACCESS_KEY`, `JWT_SECRET`, `RESEND_API_KEY`, `RESEND_FROM_EMAIL`, and `RESEND_TEMPLATE_ID` lines from `env:`, leaving only `OCI_NAMESPACE`.

- [ ] **Step 2: Trim the `--extra-vars` string**

Change:

```yaml
            --extra-vars "release_tag=${{ needs.tag.outputs.new_tag }} oci_namespace=$OCI_NAMESPACE postgres_backup_access_key_id=$POSTGRES_BACKUP_ACCESS_KEY_ID postgres_backup_secret_access_key=$POSTGRES_BACKUP_SECRET_ACCESS_KEY jwt_secret=$JWT_SECRET resend_api_key=$RESEND_API_KEY resend_from_email=$RESEND_FROM_EMAIL resend_template_id=$RESEND_TEMPLATE_ID"
```

to:

```yaml
            --extra-vars "release_tag=${{ needs.tag.outputs.new_tag }} oci_namespace=$OCI_NAMESPACE"
```

- [ ] **Step 3: Validate YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/deploy.yml'))"
```

Expected: no error.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/deploy.yml
git commit -m "feat: remove account secrets from deploy workflow"
```

---

### Task 10: End-to-end smoke test

**Files:** none (verification only)

**Interfaces:** none

- [ ] **Step 1: Start the backend**

```bash
cd backend && go run main.go
```

Expected: starts on `:8080` with no fatal errors and no mention of `JWT_SECRET`, `POSTGRES_URL`, or Resend/WebAuthn — those checks no longer exist post-Task-1.

- [ ] **Step 2: Confirm removed routes are gone**

In another terminal:

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/auth/me
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api-keys
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/roles
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/acl
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/pads/test
```

Expected: `404` for all five (no handler registered for any of these paths anymore).

- [ ] **Step 3: Confirm the pad endpoint still works**

```bash
curl -s -X PUT http://localhost:8080/pads/smoke-test -H "Content-Type: application/json" -d '{"content":"hello","encrypted":false}'
curl -s http://localhost:8080/pads/smoke-test
```

Expected: both succeed (200) and the GET returns the content written by the PUT.

- [ ] **Step 4: Start the frontend and check it in a browser**

```bash
cd frontend && pnpm dev
```

Visit `http://localhost:3000`, confirm the home page renders with no account bubble in the header, type a slug, navigate to the pad, edit it, confirm autosave still works. Visit `http://localhost:3000/_/login` and `http://localhost:3000/_/profile` and confirm both now 404 (the routes no longer exist).

- [ ] **Step 5: Run the remaining Cypress suite**

With backend and frontend still running:

```bash
cd frontend && npx cypress run
```

Expected: only `pad.cy.ts` runs (since `api-access.cy.ts` was deleted in Task 4) and it passes.

- [ ] **Step 6: Stop the backend and frontend dev servers** (Ctrl-C in each terminal)

---

### Task 11: Update `CHANGELOG.md`

**Files:**
- Modify: `CHANGELOG.md`

**Interfaces:** none

- [ ] **Step 1: Read the existing entry format**

```bash
head -20 CHANGELOG.md
```

- [ ] **Step 2: Append a new entry**

Following the format at the top of the file, add an entry summarizing this session's work: removal of the account subsystem (auth, API keys, roles, ACL, profile settings, passkeys, email verification) from `backend/`, `frontend/`, `docs/`, `.github/workflows/deploy.yml`, and `infra/`, plus the new `009_drop_accounts.sql` teardown migration. List the actual files changed across Tasks 1–9 (deletions and modifications) — Task 10 was verification-only and changed no files.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: update changelog for account removal"
```
