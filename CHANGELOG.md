# Changelog

This file is maintained by AI agents. Every time an agent makes any change to the codebase — files added, modified, or deleted — it must append an entry here before ending the session. No exceptions, even for small changes.

## Entry format

```
## YYYY-MM-DD — <short summary>

**Agent:** <model name and session context>
**Files changed:** <list of files>

**What changed:**
- <bullet: added / modified / deleted X>

**Why:** <the user's intent or task that triggered this change>
```

---

## 2026-06-14 — fix: CI/Ansible code review blockers

**Agent:** claude-sonnet-4-6
**Files changed:**
- `.gitignore` (modified)
- `infra/ansible/playbook.yml` (modified)
- `.github/workflows/deploy.yml` (modified)

**What changed:**
- Removed `infra/ansible/inventory.ini` from `.gitignore` so the file-based inventory is tracked in git (CI references it with `-i inventory.ini`)
- Added `pre_tasks` guard to the "Deploy application" play in `playbook.yml` asserting the worker IP placeholder has been replaced — fires even when `--tags deploy` skips the k3s_agent play
- Added `ANSIBLE_HOST_KEY_CHECKING: "False"` env var to the "Deploy to k3s cluster" step in `deploy.yml` to prevent SSH fingerprint mismatches from hanging the deploy

**Why:** Code review identified these as critical blockers: inventory.ini being gitignored breaks CI, the placeholder guard only ran during k3s_agent provisioning (not deploy-only runs), and missing host key checking could cause silent CI hangs.

---

## 2026-06-14 — Phase 4: Update GitHub Actions deploy workflow for k3s

**Agent:** claude-sonnet-4-6
**Files changed:**
- `.github/workflows/deploy.yml` (modified)

**What changed:**
- Removed `ansible-galaxy collection install community.docker` from the Ansible install step (no longer needed)
- Renamed install step to "Install Ansible"
- Added `WORKER_SSH_KNOWN_HOST` secret to SSH setup so the agent node host key is trusted
- Replaced "Deploy backend and frontend" step with "Deploy to k3s cluster" step:
  - Switched from inline `-i "${{ secrets.VM_IP }},"` inventory to `-i inventory.ini`
  - Changed `--tags backend,frontend` to `--tags deploy`
  - Added `oci_namespace=$OCI_NAMESPACE` to `--extra-vars` (required by k8s-manifests role)
  - Removed `-e "ansible_user=opc ansible_ssh_private_key_file=..."` (now defined in inventory.ini)

**Why:** Phase 4 task — align the CI/CD workflow with the new k3s-based Ansible playbook that uses file-based inventory and k8s deployment tags instead of the old Docker/Ansible approach.

---

## 2026-06-14 — fix: Ansible role hardening — 10 code-quality fixes

**Agent:** claude-sonnet-4-6
**Files changed:**
- `infra/ansible/playbook.yml` (modified)
- `infra/ansible/roles/k3s-agent/tasks/main.yml` (modified)
- `infra/ansible/roles/k8s-manifests/tasks/main.yml` (modified)

**What changed:**
- [playbook.yml] Added `pre_tasks` assert to the k3s-agent play to fail fast when worker IP is still the placeholder value
- [k3s-agent] Replaced `hostvars` cross-play token reference with `delegate_to` slurp + set_fact — role now self-sufficient with `--limit k3s_agent`
- [k3s-agent] Added `wait_for` task to verify k3s API port 6443 is reachable before agent join
- [k3s-agent] Changed `creates:` guard from `/usr/local/bin/k3s` to `/etc/systemd/system/k3s-agent.service` for correct idempotency
- [k3s-agent] Changed `--node-name=zeropad-worker` to `--node-name={{ inventory_hostname }}` to support multiple workers
- [k8s-manifests] Added `rsync` install task as first task so `synchronize` module has its dependency
- [k8s-manifests] Added `assert` task to validate `release_tag` and `oci_namespace` are defined before applying manifests
- [k8s-manifests] Replaced `kubectl apply -f backend/` directory apply with individual `deployment.yaml` and `service.yaml` applies to prevent ConfigMap overwrite
- [k8s-manifests] Added `--force-update` to `helm repo add` for idempotency
- [k8s-manifests] Added `KUBECONFIG` env var to "Apply namespace", "Create or update backend ConfigMap", and "Create or update frontend release-tag ConfigMap" tasks

**Why:** Code quality review identified 10 critical/important issues in the Ansible roles; all have been resolved.

---

## 2026-06-14 — Phase 3: Refactor Ansible to install k3s and deploy via kubectl/Helm

**Agent:** claude-sonnet-4-6, Phase 3 implementation
**Files changed:**
- `infra/ansible/inventory.ini` (modified)
- `infra/ansible/playbook.yml` (modified)
- `infra/ansible/roles/k3s-server/tasks/main.yml` (added)
- `infra/ansible/roles/k3s-agent/tasks/main.yml` (added)
- `infra/ansible/roles/k8s-manifests/tasks/main.yml` (added)
- `infra/ansible/roles/docker/` (deleted)
- `infra/ansible/roles/backend/` (deleted)
- `infra/ansible/roles/frontend/` (deleted)
- `infra/ansible/roles/caddy/` (deleted)

**What changed:**
- Replaced single-host `[zeropad]` inventory group with `[k3s_server]` + `[k3s_agent]` + `[k3s_cluster:children]` two-group structure; worker IP is a placeholder (`WORKER_PUBLIC_IP_PLACEHOLDER`) to be filled from `terraform output worker_public_ip`
- Replaced single-play Docker-based playbook with three plays: configure k3s servers (runs `volume` + `k3s-server` roles on all `k3s_server` hosts), configure k3s agents (runs `k3s-agent` role on all `k3s_agent` hosts), deploy application (runs `k8s-manifests` role on `k3s_server`)
- Added `k3s-server` role: disables firewalld, installs k3s server with traefik disabled, waits for API readiness, reads the node join token and exposes it as an Ansible fact (`k3s_node_token`) for cross-play consumption
- Added `k3s-agent` role: disables firewalld, joins each agent to the cluster using the server's IP and the token propagated via `hostvars`
- Added `k8s-manifests` role: rsync's `infra/k8s/` to remote, applies namespace, installs Helm + ingress-nginx, creates backend and frontend ConfigMaps, applies all backend/frontend/ingress manifests, patches image tags, waits for rollouts to complete
- Deleted `docker`, `backend`, `frontend`, and `caddy` roles (replaced by k3s + k8s-manifests approach)
- `volume` role is unchanged

**Why:** Phase 3 of k3s cluster migration — replace Docker/Caddy-based Ansible deployment with k3s installation and Kubernetes-native deploy using the `infra/k8s/` manifests created in Phase 2.

---

## 2026-06-14 — Phase 2: add infra/k8s/ Kubernetes manifests directory

**Agent:** claude-sonnet-4-6, Phase 2 implementation
**Files changed:**
- `infra/k8s/namespace.yaml` (added)
- `infra/k8s/backend/configmap.yaml` (added)
- `infra/k8s/backend/deployment.yaml` (added)
- `infra/k8s/backend/service.yaml` (added)
- `infra/k8s/frontend/nginx-configmap.yaml` (added)
- `infra/k8s/frontend/deployment.yaml` (added)
- `infra/k8s/frontend/service.yaml` (added)
- `infra/k8s/ingress/ingress-nginx-values.yaml` (added)
- `infra/k8s/ingress/ingress.yaml` (added)
- `infra/k8s/secrets/README.md` (added)

**What changed:**
- Added `zeropad` namespace manifest
- Added backend ConfigMap, Deployment (2 replicas, readiness probe on `/health`), and ClusterIP Service
- Added frontend Deployment with alpine init container that fetches the tarball from GitHub Releases, nginx ConfigMap replicating Caddy SPA/txt fallback routing, and ClusterIP Service
- Added NGINX Ingress Controller Helm values for bare-metal DaemonSet with hostNetwork
- Added Ingress routing `zeropad.dev` → frontend and `api.zeropad.dev` → backend with Cloudflare origin TLS
- Added secrets/README.md documenting the one-time `kubectl create secret tls` bootstrap step

**Why:** Greenfield creation of Kubernetes manifests to enable k3s-based deployment, replacing the current Ansible-only VM deployment approach.

---

## 2026-06-14 — fix: add livenessProbe, CPU limit to backend and readinessProbe to frontend k8s manifests

**Agent:** claude-sonnet-4-6, code review fix
**Files changed:**
- `infra/k8s/backend/deployment.yaml`
- `infra/k8s/frontend/deployment.yaml`

**What changed:**
- Added `livenessProbe` (httpGet `/health:8080`, initialDelay 15s, period 20s, failureThreshold 3) to backend container
- Added `cpu: "500m"` to backend `resources.limits` (was memory-only)
- Added `readinessProbe` (httpGet `/:80`, initialDelay 3s, period 5s) to frontend nginx container

**Why:** Code review identified three critical quality issues: missing liveness probe on backend, missing CPU resource limit on backend, and missing readiness probe on frontend nginx container.

---

## 2026-06-14 — Terraform: surface worker_private_ip root output and fix dynamic group description

**Agent:** claude-sonnet-4-6, code review fix
**Files changed:**
- `infra/terraform/outputs.tf`
- `infra/terraform/modules/compute/main.tf`

**What changed:**
- Added `worker_private_ip` root-level output in `outputs.tf`, surfacing `module.compute.worker_private_ip` for Ansible k3s join configuration
- Fixed `oci_identity_dynamic_group.backend` `description` field: "VM" → "VMs" (plural, matches the matching rule which covers both instances)

**Why:** Code review identified the root outputs file was missing `worker_private_ip` (needed by Ansible for k3s cluster join) and a minor description typo in the dynamic group resource.

---

## 2026-06-14 — Terraform: add k3s worker node (Node 2) and update networking

**Agent:** claude-sonnet-4-6, Phase 1 Terraform changes
**Files changed:**
- `infra/terraform/modules/compute/main.tf`
- `infra/terraform/modules/compute/variables.tf`
- `infra/terraform/modules/compute/outputs.tf`
- `infra/terraform/modules/networking/main.tf`
- `infra/terraform/main.tf`
- `infra/terraform/variables.tf`
- `infra/terraform/outputs.tf`

**What changed:**
- Added `oci_core_instance.worker` resource (2 OCPU / 12 GB RAM ARM VM) as k3s worker node
- Updated IAM dynamic group `matching_rule` to include both Node 1 and Node 2 instance IDs
- Added `worker_vm_ocpus` and `worker_vm_memory_gbs` variables to compute module, root variables, and root module call
- Added `worker_public_ip` and `worker_private_ip` outputs to compute module
- Added `worker_public_ip` and `worker_ssh_command` outputs to root outputs
- Downsized Node 1 defaults: `vm_ocpus` 4→2, `vm_memory_gbs` 8/24→12 (module/root)
- Added three internal-only ingress security rules to networking: k3s API server (TCP 6443), Flannel VXLAN (UDP 8472), kubelet API (TCP 10250) — all scoped to VCN CIDR 10.0.0.0/16

**Why:** Phase 1 of k3s Kubernetes cluster setup — provision a second OCI ARM VM as a worker node and open the necessary inter-node ports for cluster communication.

---

## 2026-06-14 — Fix /health handler: method guard and error logging

**Agent:** claude-sonnet-4-6, code quality fix
**Files changed:** `backend/main.go`

**What changed:**
- Changed `/health` route pattern to `GET /health` (Go 1.22+ method-qualified mux pattern) so non-GET requests automatically receive `405 Method Not Allowed`
- Assigned the return value of `json.NewEncoder(w).Encode(...)` and logged any error via `log.Printf`, matching the `writeJSON` convention used elsewhere in the codebase

**Why:** Code review identified two issues: silently discarded JSON encode errors and no HTTP method guard on the health endpoint.

---

## 2026-06-14 — Add /health endpoint to Go backend

**Agent:** claude-sonnet-4-6, Phase 0 infra task
**Files changed:** `backend/main.go`

**What changed:**
- Added `GET /health` handler on the existing `mux`, wrapped with the `cors` middleware, returning HTTP 200 with `{"status":"ok"}`
- Added `encoding/json` import to support inline JSON encoding

**Why:** Prerequisite for Kubernetes liveness/readiness probes in the upcoming backend deployment manifest.

---

## 2026-05-23 — Add pad ownership, presentation mode, marketplace, and no-KYC principle to roadmap

**Agent:** claude-sonnet-4-6, monetization planning session
**Files changed:** `docs/roadmap.md` (modified)

**What changed:**
- Added "Core principles" section establishing no-KYC and privacy-first as hard constraints
- Updated Phase 1.1 table list: added `pad_bids`; noted `presentation_mode`, `for_sale`, `min_bid_*` fields in `pad_meta`
- Added Phase 3.3 Pad claiming — paid user can take ownership of anonymous/free-owned pads
- Added Phase 3.4 Presentation mode — owner can make a pad publicly readable but not editable
- Added Phase 3.5 Pad marketplace — paid users can list pads for sale and receive crypto bids; v1 wallet-to-wallet, v2 escrow smart contract
- Replaced Phase 3.6 Stripe billing with Phase 3.9 crypto-native billing (ETH/USDC/Lightning/Monero, no KYC)
- Renumbered Phase 3.3–3.6 → 3.6–3.9 to accommodate new sections
- Fixed Phase 4.1 team invite to use username/wallet instead of email
- Fixed Phase 5 to remove Stripe and email references

**Why:** User defined no-KYC as a core product principle and requested pad claiming, presentation mode, and a crypto-native pad marketplace.

---

## 2026-05-23 — Update roadmap auth to use random ID and SIWE/SIWX

**Agent:** claude-sonnet-4-6, monetization planning session
**Files changed:** `docs/roadmap.md` (modified)

**What changed:**
- Replaced email + password auth in Phase 1.3 with two passwordless methods: random UUID identity and SIWE/SIWX wallet login

**Why:** User wants auth to be passwordless — either a randomly generated ID the user saves, or wallet-based login via SIWE (already implemented for key derivation).

---

## 2026-05-23 — Update roadmap with SQLite-on-Block-Volume storage approach

**Agent:** claude-sonnet-4-6, monetization planning session
**Files changed:** `docs/roadmap.md` (modified)

**What changed:**
- Rewrote Phase 1 storage section: replaced generic "add a database" with concrete SQLite-on-OCI-Block-Volume plan (Option A)
- Added full SQLite schema for all tables: `users`, `sessions`, `pad_meta`, `namespaces`, `api_keys`, `api_usage`, `teams`, `team_members`, `audit_log`
- Added Phase 1.2: OCI Object Storage lifecycle rule for free-tier TTL (tags `tier=free`/`tier=paid` at upload; Oracle handles deletion — no background job)
- Expanded all phases with concrete implementation details reflecting the OCI + SQLite dual-store architecture

**Why:** User confirmed Option A (SQLite on existing Block Volume + OCI Object Storage for content) as the storage strategy; roadmap updated to reflect the actual infra.

---

## 2026-05-23 — Add freemium monetization roadmap

**Agent:** claude-sonnet-4-6, monetization planning session
**Files changed:** `docs/roadmap.md` (created)

**What changed:**
- Added `docs/roadmap.md` — a phased implementation roadmap for Free / Pro / Team freemium tiers

**Why:** User requested a structured roadmap for monetizing dopad with freemium limits across three tiers (Free, Pro at $8/mo, Team at $24/mo), ordered by implementation priority.

---

## 2026-05-22 — Content type selector (Text, Rich Text, LaTeX, Code)

**Agent:** claude-sonnet-4-6
**Files changed:**
- `frontend/app/[slug]/contentTypes.ts` — added `ContentType`, `Language` types, `CONTENT_TYPES`/`LANGUAGES` registries, `PadContentEnvelope`, `parseContent`, `serializeContent`
- `frontend/app/[slug]/ContentTypeSelect.tsx` — pill-button segmented control for choosing content type
- `frontend/app/[slug]/LanguageSelect.tsx` — pill-button segmented control for choosing language (shown when type is "code")
- `frontend/app/[slug]/RichTextEditor.tsx` — TipTap-based rich text editor
- `frontend/app/[slug]/LatexEditor.tsx` — split-pane LaTeX editor (raw textarea left, KaTeX-rendered preview right)
- `frontend/app/[slug]/CodeEditor.tsx` — CodeMirror 6 editor with JavaScript, C (via lang-cpp), and Python support; dark/light theme aware
- `frontend/app/[slug]/PadEditor.tsx` — added `contentType`, `language`, `body` state; toolbar row below header with `ContentTypeSelect` + conditional `LanguageSelect`; conditional editor rendering; `handleBodyChange` serializes envelope before auto-save; unlock handlers parse content after decryption
- `frontend/package.json` — added `@tiptap/react`, `@tiptap/pm`, `@tiptap/starter-kit`, `katex`, `@uiw/react-codemirror`, `@codemirror/lang-javascript`, `@codemirror/lang-cpp`, `@codemirror/lang-python`, `@codemirror/theme-one-dark`; added `@types/katex` dev dep

**What changed:**
- Pad content is now stored as a JSON envelope `{ type, lang?, body }` — no backward compat with old plain-text pads (data was wiped)
- A toolbar appears between the header and the editor area with four pill buttons: Text / Rich Text / LaTeX / Code
- Selecting Code reveals a second pill group: JavaScript / C / Python
- Each type renders a different editor component; all editors feed into the same debounced auto-save and encryption flow

**Why:** Users needed to choose a content mode (plain text, rich text, LaTeX, or code with syntax highlighting) when creating or editing a pad.

---

## 2026-05-22 — Write-token authentication for encrypted pads

**Agent:** claude-sonnet-4-6
**Files changed:**
- `backend/middlewares/cors.go` — added `X-Write-Token` to allowed CORS headers
- `backend/adapters/store/pad.go` — added `HashedWriteToken` field to `Pad` struct
- `backend/adapters/http/pad.go` — two-phase GET (metadata-only vs full response), write token validation on PUT, `sha256Hex` helper, `padMetaResponse` struct
- `frontend/app/_lib/crypto.ts` — extended `KeyDeriver` interface with `deriveWriteToken`; added `writeTokenFromKeyMaterial` helper; updated `getPasswordDeriver` and refactored `SIWEKeyDeriver` with keyMaterial cache to avoid double wallet popup; updated registry sentinel
- `frontend/app/_lib/pads.ts` — added `writeToken`/`newWriteToken` to `PadData`; added `getPadContent` (authenticated GET with `X-Write-Token` header); updated `setPad` to send token fields and handle 403
- `frontend/app/[slug]/PadEditor.tsx` — added `writeTokenRef`; updated unlock handlers to derive token and call `getPadContent`; updated auto-save to include write token; updated encrypt/re-encrypt handlers to send old and new tokens on key change
- `docs/api-spec.md` — documented two-phase GET responses, new body fields, and 403 responses for both endpoints
- `docs/architecture.md` — updated Key Derivation section and Encryption Data Flow (write + read paths)
- `docs/features.md` — added Write-Token Authentication section; clarified verify_blob role

**What changed:**
- `GET /pads/{slug}` on an encrypted pad without `X-Write-Token` now returns only `{ slug, encrypted, deriver_id }` — no ciphertext or verify_blob
- `GET /pads/{slug}` with a valid `X-Write-Token` header returns the full response; wrong token returns 403
- `PUT /pads/{slug}` on an existing encrypted pad requires `write_token` in the body; wrong or missing token returns 403
- Key changes send both `write_token` (old) and `new_write_token` (new); server validates old and replaces stored hash with SHA-256 of new
- Server stores `sha256(write_token)` — never the token itself; the AES-GCM key never leaves the browser
- Unencrypted pads and legacy encrypted pads (no stored token) are unaffected

**Why:** Encrypted pads were fully writable and readable by anyone who knew the slug. Write tokens derived from the same key material as the AES-GCM encryption key now gate both reads and writes on encrypted pads.

---

## 2026-05-22 — Horizontal scaling with multiple backend containers and Caddy load balancing

**Agent:** claude-sonnet-4-6
**Files changed:**
- `infra/ansible/playbook.yml` — added `backend_replicas`, `backend_memory`, `backend_memory_reservation` vars
- `infra/ansible/roles/backend/tasks/main.yml` — replaced single container task with N-replica loop; added legacy container removal; added memory limits
- `infra/ansible/roles/caddy/templates/Caddyfile.j2` — replaced hard-coded `reverse_proxy localhost:8080` with dynamic round-robin upstream list

**What changed:**
- Backend now starts `backend_replicas` containers (default: 2) named `zeropad-backend-0`, `zeropad-backend-1`, etc., on sequential ports (8080, 8081, …)
- Each container has a 256 MB hard memory limit and 128 MB soft reservation
- Caddy load balances across all replicas using round-robin
- Legacy `zeropad-backend` container (no suffix) is stopped and removed on first deploy
- All scaling knobs (`backend_replicas`, `backend_memory`, `backend_memory_reservation`) are playbook vars

**Why:** Scale the backend horizontally to handle more traffic on the single OCI VM

---

## 2026-05-22 — Add production deploy pipeline (workflow_dispatch with version bump)

**Agent:** claude-sonnet-4-6
**Files changed:**
- `.github/workflows/deploy.yml` — added (new manual deploy workflow)
- `infra/ansible/playbook.yml` — modified (role-level tags for selective execution)
- `infra/ansible/roles/backend/tasks/main.yml` — modified (added `recreate: true` to docker_container task)

**What changed:**
- New `Deploy to Production` GitHub Actions workflow with `workflow_dispatch` trigger
- Inputs: `version_bump` dropdown (patch/minor/major) and `release_notes` text field
- Three sequential jobs: `tag` (bumps semver, creates annotated git tag), `build` (Docker image + frontend tarball + GitHub Release), `deploy` (Ansible with `--tags backend,frontend`)
- Ansible playbook roles now have explicit tags so only backend/frontend are re-run on deploy
- Backend `docker_container` task uses `recreate: true` to always pick up newly pulled image

**Why:** No automated way to create releases or deploy to production; required manually tagging, building, and SSHing to update the server.

---

## 2026-05-21 — Replace in-memory pad store with OCI Object Storage

**Agent:** claude-sonnet-4-6
**Files changed:**
- `backend/adapters/store/oci.go` — added (OCIPadStore implementing PadStore via OCI Object Storage)
- `backend/adapters/store/pad.go` — modified (added json tags to Pad struct)
- `backend/main.go` — modified (added selectStore() — uses OCIPadStore when OCI_BUCKET_NAME+OCI_NAMESPACE set, else MemoryPadStore)
- `backend/go.mod` + `backend/go.sum` — modified (added github.com/oracle/oci-go-sdk/v65, go 1.21→1.24)
- `infra/terraform/modules/compute/main.tf` — modified (added oci_identity_dynamic_group for Instance Principal auth)
- `infra/terraform/modules/compute/variables.tf` — modified (added tenancy_ocid variable)
- `infra/terraform/modules/compute/outputs.tf` — modified (added backend_dynamic_group_name output)
- `infra/terraform/main.tf` — modified (added oci_identity_policy for bucket access, pass tenancy_ocid to compute module)
- `infra/ansible/roles/backend/templates/zeropad.env.j2` — modified (added OCI_BUCKET_NAME and OCI_NAMESPACE)
- `infra/ansible/playbook.yml` — modified (added oci_namespace var from OCI_NAMESPACE env)

**What changed:**
- Added OCIPadStore backed by `zeropad-pads` OCI Object Storage bucket; each pad stored as JSON object named by slug
- Instance Principal auth (no credentials in container) with API key env var fallback for local dev
- selectStore() in main.go picks OCI store when env vars present, falls back to memory for local dev
- Terraform provisions IAM dynamic group + policy granting the VM least-privilege access to bucket objects
- Ansible template now injects OCI_BUCKET_NAME and OCI_NAMESPACE into the backend container env

**Why:** Pads were lost on server restart; connect the already-provisioned OCI bucket to make storage persistent.

---

## 2026-05-17 — Add Terraform + Ansible IaC for OCI Always Free deployment (zeropad.dev)

**Agent:** claude-sonnet-4-6
**Files changed:**
- `backend/main.go` — modified (read `ALLOW_ORIGIN` env var, default `http://localhost:3000`)
- `backend/Dockerfile` — modified (`GOARCH=amd64` → `GOARCH=arm64` for Ampere A1 VM)
- `.github/workflows/release.yml` — modified (add `platforms: linux/arm64` to Docker build)
- `infra/terraform/.gitignore` — added
- `infra/terraform/terraform.tfvars.example` — added
- `infra/terraform/variables.tf` — added
- `infra/terraform/main.tf` — added (OCI + Cloudflare providers, image datasource)
- `infra/terraform/networking.tf` — added (VCN, IGW, route table, security list, subnet)
- `infra/terraform/storage.tf` — added (50 GB block volume, `zeropad-pads` Object Storage bucket)
- `infra/terraform/compute.tf` — added (Ampere A1 VM + block volume attachment)
- `infra/terraform/loadbalancer.tf` — added (OCI Flexible LB 10 Mbps, backend set, HTTP listener)
- `infra/terraform/cloudflare.tf` — added (A record + www CNAME for `zeropad.dev`, proxied)
- `infra/terraform/outputs.tf` — added
- `infra/ansible/inventory.ini` — added
- `infra/ansible/playbook.yml` — added
- `infra/ansible/roles/volume/tasks/main.yml` — added (format + mount block volume to /data)
- `infra/ansible/roles/docker/tasks/main.yml` — added (install Docker CE on Oracle Linux 9)
- `infra/ansible/roles/backend/tasks/main.yml` — added (pull + run dopad-backend container)
- `infra/ansible/roles/backend/templates/zeropad.env.j2` — added

**Why:** User requested IaC for OCI Always Free tier ($0/month): 1 Ampere A1 VM, OCI Flexible LB, Object Storage bucket, 50 GB block volume, Cloudflare CDN/DNS for `zeropad.dev`. Terraform provisions infrastructure; Ansible configures the server.

---

## 2026-05-14 — Add GitHub Releases CI/CD pipeline with Docker image and static frontend bundle

**Agent:** claude-sonnet-4-6
**Files changed:**
- `.github/workflows/release.yml` — added (GitHub Actions release workflow)
- `backend/Dockerfile` — added (multi-stage Go build → distroless runtime)
- `frontend/next.config.ts` — modified (added `output: "export"` for static export)
- `frontend/app/[[...slug]]/page.tsx` — added (server component with `generateStaticParams`)
- `frontend/app/[[...slug]]/PageContent.tsx` — added (client component: home + pad routing)
- `frontend/app/[[...slug]]/PadEditor.tsx` — added (moved from `app/[slug]/PadEditor.tsx`)
- `frontend/app/[[...slug]]/DeriverSelect.tsx` — added (moved from `app/[slug]/DeriverSelect.tsx`)
- `frontend/app/[slug]/page.tsx` — deleted (replaced by catch-all route)
- `frontend/app/[slug]/PadEditor.tsx` — deleted (moved to `[[...slug]]`)
- `frontend/app/[slug]/DeriverSelect.tsx` — deleted (moved to `[[...slug]]`)

**What changed:**
- Added `.github/workflows/release.yml`: on tag push matching `v*.*.*`, builds a Docker image for the backend and pushes it to `ghcr.io/{owner}/dopad-backend:{tag}` + `:latest`; builds the frontend static export and attaches `frontend-{tag}.tar.gz` to the GitHub Release
- Added `backend/Dockerfile`: two-stage build using `golang:1.21-alpine` builder and `gcr.io/distroless/static-debian12` runtime; binary compiled with `CGO_ENABLED=0 GOOS=linux` and `-ldflags="-w -s"` for a minimal, static binary
- Enabled `output: "export"` in `next.config.ts` so `pnpm build` emits a deployable `out/` directory of static HTML/CSS/JS
- Refactored routing from `app/page.tsx` + `app/[slug]/` to a single `app/[[...slug]]/` optional catch-all; `generateStaticParams` produces one template at `/_`; CDN serves `_.html` for all paths and client-side `useParams()` resolves the real slug at runtime

**Why:** User requested a GitHub Releases integration to produce versioned artifacts: a Docker image for the Go backend and a compressed static bundle for the Next.js frontend, to be deployed later to a cloud environment.

---

## 2026-05-14 — Rename Go module from no-trust-cms-backend to dopad-backend

**Agent:** claude-sonnet-4-6
**Files changed:**
- `backend/go.mod` — module declaration updated
- `backend/main.go` — 4 import paths updated
- `backend/adapters/http/pad.go` — 3 import paths updated
- `backend/adapters/store/pad.go` — 1 import path updated
- `backend/services/pad/service.go` — 1 import path updated

**Why:** Project renamed from no-trust-cms to dopad; all Go import paths must match the module name in go.mod.

---

## 2026-05-14 — Replace method-picker tabs with shared DeriverSelect dropdown; fix layout shift

**Agent:** claude-sonnet-4-6
**Files changed:**
- `frontend/app/[slug]/DeriverSelect.tsx` — new file; `DeriverSelect` component renders a `<select>` driven by `keyDerivers` registry
- `frontend/app/[slug]/PadEditor.tsx` — replaced both inline tab-button blocks (lock screen + header encrypt form) with `<DeriverSelect>`; wrapped dynamic form content in a fixed-width `w-64` / `w-72` container to prevent layout shift when switching methods

**Why:** The two-button pill tabs caused the picker to jump left/right when the user switched methods because the adjacent form (two inputs vs one button) had different widths. A `<select>` dropdown with a fixed-width slot for the form content keeps the picker pinned. Extracting a shared component eliminates duplication between the lock screen and the header encrypt form.

---

## 2026-05-14 — Store deriver_id on pads; auto-select unlock method on locked pad screen

**Agent:** claude-sonnet-4-6
**Files changed:**
- `backend/encryption/deriver.go` — new file; `Deriver` string type with `DeriverPassword = "password"` and `DeriverSIWE = "siwe"` constants
- `backend/adapters/store/pad.go` — added `DeriverId encryption.Deriver` field to `Pad` struct
- `backend/adapters/http/pad.go` — added `deriver_id` to PUT request body and to `padResponse` (returned by both GET and PUT)
- `frontend/app/_lib/crypto.ts` — exported `DERIVER_PASSWORD`, `DERIVER_SIWE` constants and `DeriverId` type; updated registry and class to use them
- `frontend/app/_lib/pads.ts` — added `deriverId: DeriverId | ""` to `PadData`; maps `deriver_id` ↔ `deriverId` in GET/PUT
- `frontend/app/[slug]/PadEditor.tsx` — added `deriverId` state; `useEffect` reads `pad.deriverId` and auto-sets `selectedMethod` on locked pads; all `setPad` calls include `deriverId`

**Why:** When a user returned to a pad they encrypted with a wallet (SIWE), the lock screen defaulted to the Password tab, forcing a manual tab switch. Persisting `deriver_id` and using it to initialize `selectedMethod` fixes the UX.

---

## 2026-05-14 — Remove SIWE auth + add extensible KeyDeriver abstraction for pad encryption

**Agent:** claude-sonnet-4-6
**Files changed:**
- `backend/adapters/store/nonce.go` — deleted
- `backend/services/auth/service.go` — deleted (entire `services/auth/` directory removed)
- `backend/adapters/http/auth.go` — deleted
- `backend/adapters/http/pad.go` — moved `writeJSON` helper here (was in auth.go)
- `backend/main.go` — removed auth wiring, nonce store, JWT secret, sweep goroutine
- `backend/middlewares/cors.go` — removed `Authorization` from allowed headers; removed `POST` from allowed methods
- `backend/go.mod` / `backend/go.sum` — removed `golang-jwt/jwt`, `spruceid/siwe-go`, and all transitive deps
- `frontend/app/_components/login.tsx` — deleted
- `frontend/app/_lib/api.ts` — removed `SESSION_KEY` export and Bearer token injection
- `frontend/app/_lib/crypto.ts` — appended `KeyDeriver` interface, `getPasswordDeriver` factory, `SIWEKeyDeriver` class, `keyDerivers` registry, `getDeriver`
- `frontend/app/[slug]/PadEditor.tsx` — added method picker tabs (Password / Wallet SIWE) on lock screen and encrypt form; `handleSIWEUnlock`, `handleSIWEFormEncrypt`
- `frontend/package.json` / `frontend/pnpm-lock.yaml` — removed `siwe` npm package
- `docs/architecture.md` — removed Auth Flow section; updated component tables; added Key Derivation section
- `docs/api-spec.md` — removed all `/auth/*` endpoints; updated CORS conventions
- `docs/features.md` — replaced Authentication section with Encryption section listing both derivers

**Why:** The SIWE authentication flow (wallet login → JWT) is no longer needed. The nonce store, which backed that flow, was dead code. SIWE wallet signing is repurposed as a client-side encryption key derivation method alongside password-based derivation. A `KeyDeriver` interface and registry make it straightforward to add more methods (Google Auth, Microsoft Auth, etc.) in the future.

---

## 2026-05-14 — Fix: encrypted pad shows ciphertext on reload instead of password prompt

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `frontend/app/[slug]/PadEditor.tsx` (modified)

**What changed:**
- Added `clearTimeout(timerRef.current)` at the top of `handlePasswordFormSubmit` to cancel any pending auto-save debounce timer before the encryption save begins

**Why:** Race condition — a debounce timer set while the user was typing could fire while the encryption `setPad` call was in-flight. If that auto-save network request resolved after the encryption save, it would overwrite the backend with `{encrypted: false}`, causing the ciphertext to be shown in the textarea on the next reload instead of the password-unlock overlay.

---

## 2026-05-14 — Add password-based pad encryption (AES-GCM-256 + SHA3-256)

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `backend/adapters/store/pad.go` (modified)
- `backend/services/pad/service.go` (modified)
- `backend/adapters/http/pad.go` (modified)
- `frontend/app/_lib/crypto.ts` (added)
- `frontend/app/_lib/pads.ts` (modified)
- `frontend/app/[slug]/PadEditor.tsx` (modified)
- `frontend/package.json` (modified — added `@noble/hashes`)

**What changed:**
- Backend `Pad` struct extended with `Encrypted bool` and `VerifyBlob string`; `PadStore` interface and `MemoryPadStore` updated accordingly
- `pad.Service.Get/Set` now operate on `store.Pad` instead of plain strings
- HTTP `GET /pads/{slug}` response now includes `encrypted` and `verify_blob` fields; `PUT` body accepts the same
- New `frontend/app/_lib/crypto.ts`: `deriveKey` (SHA3-256 password → AES-GCM-256 key), `encryptText`/`decryptText` (AES-GCM, base64 IV+ciphertext), `makeVerifyBlob`/`checkVerifyBlob` (sentinel + salt approach for cheap wrong-key detection)
- `pads.ts` updated: new `PadData` type, `getPad` returns `PadData | null`, `setPad` accepts `PadData` with encryption fields
- `PadEditor.tsx` rewritten with `loading → locked → unlocked` state machine: password overlay for locked pads, "Encrypt" button for unencrypted pads, "Change password" button for already-encrypted pads; auto-save re-encrypts with the session key held in memory

**Why:** Implement client-side end-to-end encryption so the server never sees plaintext pad content.

---

## 2026-05-08 — Fix review-docs errors and warnings

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `backend/adapters/http/pad.go` (modified)
- `backend/middlewares/ratelimit.go` (modified)
- `frontend/app/[slug]/PadEditor.tsx` (modified)
- `docs/architecture.md` (modified)
- `docs/code-style.md` (modified)

**What changed:**
- `pad.go`: replaced `_ = h.svc.Set(...)` with proper error handling — logs and returns 500 on failure
- `pad.go`: added missing `"log"` import
- `ratelimit.go`: acknowledged `w.Write` return value with `_, _ =`
- `PadEditor.tsx`: replaced `<a href="/">` with `<Link href="/">` from `next/link` (ESLint no-html-link-for-pages)
- `architecture.md`: updated frontend tree (`Login.tsx`, added `pads.ts`, `[slug]/` route) and backend tree (added pad service/store/handler, rate limiter; removed deleted `ports.go`)
- `code-style.md`: updated Go handler CORS rule to reflect middleware composition pattern instead of old `cors(w, r)` call-per-handler pattern

**Why:** Fixes flagged by `/review-docs` — 2 errors and 4 warnings.

---

## 2026-05-08 — Wire frontend to backend + add Cypress E2E tests

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `frontend/app/_lib/pads.ts` (rewritten)
- `frontend/app/[slug]/PadEditor.tsx` (modified)
- `frontend/cypress.config.ts` (added)
- `frontend/cypress/e2e/pad.cy.ts` (added)
- `frontend/package.json` (cypress devDependency added)

**What changed:**
- Replaced `localStorage` mock in `pads.ts` with real `apiFetch` calls to `GET /pads/{slug}` and `PUT /pads/{slug}`; 404 returns empty string, 429 throws a typed error
- Added `"rate-limited"` save state to `PadEditor`; catches 429 from `setPad` and shows amber "slow down — rate limited" message in header
- Added `cypress.config.ts` pointing to `http://localhost:3000`
- Added three-tab Cypress E2E flow: Tab 1 creates a pad via home page, Tab 2 reads it back in a fresh page load, Tab 3 edits it and verifies persistence

**Why:** User requested frontend–backend integration and Cypress E2E orchestration for the create → read → edit pad flow.

---

## 2026-05-08 — Add pad GET/PUT routes with per-IP rate limiting

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `backend/adapters/store/pad.go` (added)
- `backend/services/pad/service.go` (added)
- `backend/adapters/http/pad.go` (added)
- `backend/middlewares/ratelimit.go` (added)
- `backend/middlewares/cors.go` (modified)
- `backend/main.go` (modified)
- `docs/api-spec.md` (modified)

**What changed:**
- Added `PadStore` interface + `MemoryPadStore` (RWMutex, interface-ready for Postgres swap)
- Added `pad.Service` with `Get` (returns `ErrNotFound`) and `Set` methods
- Added `PadHandler`: `GET /pads/{slug}` and `PUT /pads/{slug}`; CORS on all methods, rate limiter on PUT only
- Added token-bucket per-IP rate limiter middleware (10 writes/min); reads `X-Forwarded-For` for proxied requests
- Updated CORS allowed methods to include `PUT`
- Wired pad layer into `main.go` alongside the existing auth handler
- Added pad endpoints to `docs/api-spec.md`

**Why:** User requested backend pad storage routes with a throttle strategy for writes, and api-spec update.

---

## 2026-05-08 — Add home page slug input and pad edit page

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `frontend/app/page.tsx` (rewritten)
- `frontend/app/[slug]/page.tsx` (added)
- `frontend/app/[slug]/PadEditor.tsx` (added)
- `frontend/app/_lib/pads.ts` (added)

**What changed:**
- Replaced home page (SIWE login) with a slug input form — user types a pad name and is redirected to `/{slug}`
- Added dynamic route `app/[slug]/page.tsx` — server component shell that awaits `params` and renders the editor
- Added `PadEditor.tsx` — full-height textarea with 800ms debounce auto-save and save status indicator in header
- Added `_lib/pads.ts` — mocked pad store using `localStorage`; async signatures so API call sites won't need to change when the real backend is wired in

**Why:** User requested the two core product pages: home (navigate to a pad) and edit (read/write pad content), with all backend calls mocked.

---

## 2026-05-08 — Move NonceStore interface to adapters/store

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `backend/services/auth/ports.go` (deleted)
- `backend/adapters/store/nonce.go` (modified)
- `backend/services/auth/service.go` (modified)

**What changed:**
- Deleted `ports.go` — interface no longer lives in the service layer
- Added `NonceStore` interface to `adapters/store/nonce.go`, co-located with its implementation
- Updated `service.go` to import `adapters/store` and reference `store.NonceStore`

**Why:** User preference — the interface should be close to the adapter that implements it, not in the service package.

---

## 2026-05-08 — Refactor Go backend into services / adapters / middlewares

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `backend/main.go` (rewritten)
- `backend/services/auth/ports.go` (added)
- `backend/services/auth/service.go` (added)
- `backend/adapters/store/nonce.go` (added)
- `backend/adapters/http/auth.go` (added)
- `backend/middlewares/cors.go` (added)
- `docs/architecture.md` (updated backend section)

**What changed:**
- Split single `main.go` into a layered architecture: services, adapters (inward HTTP + outward store), and middlewares
- `services/auth` holds all business logic (nonce generation, SIWE verification, JWT issuance/validation) behind a `NonceStore` interface
- `adapters/store` provides `MemoryNonceStore` implementing that interface; `GetAndDelete` is atomic to prevent nonce replay
- `adapters/http` provides thin HTTP handlers that decode requests, call the service, and encode responses; uses sentinel errors (`ErrInvalidSIWEMessage`, `ErrSignatureInvalid`, `ErrNonceExpired`) for precise status codes
- `middlewares/cors` is now a standard `func(HandlerFunc) HandlerFunc` wrapper instead of a boolean helper
- `main.go` is now a pure wiring file: instantiate store → inject into service → register handlers with middleware
- Updated `docs/architecture.md` to document the new directory structure and layer responsibilities

**Why:** User requested an architectural refactor separating the codebase into services (business logic), adapters (inward HTTP + outward store), and pluggable middlewares.

---

## 2026-05-08 — Fix code style violations found by /review-docs

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `frontend/app/_components/login.tsx` → renamed to `Login.tsx`
- `frontend/app/_components/Login.tsx` (modified)
- `frontend/app/page.tsx` (modified)

**What changed:**
- Renamed `login.tsx` to `Login.tsx` to match the PascalCase component naming convention
- Changed `Login` from `export default` to named `export function Login` (non-page components must use named exports)
- Updated import in `page.tsx` from default to named: `import { Login } from "./_components/Login"`
- Updated SIWE statement from `"Sign in to no-trust-cms"` to `"Sign in to dopad"`

**Why:** Errors flagged by `/review-docs` spec review — component filename, export style, and branding string all violated `docs/code-style.md`.

---

## 2026-05-08 — Initial AI playbook and project documentation

**Agent:** Claude Sonnet 4.6
**Files changed:**
- `docs/architecture.md` (added)
- `docs/api-spec.md` (added)
- `docs/features.md` (added)
- `docs/code-style.md` (added)
- `CHANGELOG.md` (added)
- `CLAUDE.md` (added)
- `.claude/skills/review-docs.md` (added)
- `.claude/skills/test.md` (added)

**What changed:**
- Added `docs/` folder with architecture overview, API spec (current endpoints only), feature list, and code style rules for Go and TypeScript
- Added root `CLAUDE.md` as the AI playbook with project context, dev commands, conventions, and the changelog rule
- Added `CHANGELOG.md` (this file) to track all agent-made changes
- Added `.claude/skills/review-docs.md` — skill that reads `docs/` and reviews changed files against the specs
- Added `.claude/skills/test.md` — skill that runs Go tests, frontend tests, and Cypress E2E integration tests

**Why:** User requested an AI playbook documenting the dopad project architecture, plus a code review skill and a test run skill.

---

## 2026-05-19 — Fix window access during static prerendering

**Agent:** claude-sonnet-4-6
**Files changed:** `frontend/app/[slug]/PadPageClient.tsx`

**What changed:**
- Replaced direct `window.location.pathname` call with `useEffect` + `useState` to defer slug resolution to client-side hydration

**Why:** Next.js static export prerendered `/_` and threw `ReferenceError: window is not defined` because `window` is not available at build time.

---

## 2026-05-19 — Rebrand UI from dopad to zeropad

**Agent:** claude-sonnet-4-6
**Files changed:** `frontend/app/page.tsx`, `frontend/app/[slug]/PadEditor.tsx`

**What changed:**
- Updated h1 on home page from "dopad" to "zeropad"
- Updated both header Link labels in PadEditor from "dopad" to "zeropad"

**Why:** User requested the frontend display "zeropad" instead of "dopad". Crypto protocol constants in crypto.ts left unchanged to preserve compatibility with existing encrypted pads.

---

## 2026-05-20 — Rename crypto sentinel strings from dopad to zeropad

**Agent:** claude-sonnet-4-6
**Files changed:** `frontend/app/_lib/crypto.ts`

**What changed:**
- Renamed `"dopad-verified:"` sentinel to `"zeropad-verified:"` in `makeVerifyBlob` and `checkVerifyBlob`
- Renamed `"dopad encrypt:"` SIWE signing message prefix to `"zeropad encrypt:"`

**Why:** App not yet in production; safe to update protocol constants to match the zeropad brand.
