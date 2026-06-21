# CloudNativePG Metadata Store Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy a 3-node HA PostgreSQL cluster via CloudNativePG on OCI k3s, with WAL archiving to a dedicated OCI bucket and a new Go metadata adapter wired into the backend.

**Architecture:** A second k3s worker VM is provisioned via Terraform and registered as an agent, giving 3 schedulable nodes. CloudNativePG runs a 3-instance cluster (1 primary + 2 standbys) spread across those nodes via anti-affinity rules. The Go backend gets a new `adapters/db` package that opens a `pgxpool` connection and runs an embedded migration for the `users` table.

**Tech Stack:** Terraform (OCI provider), Ansible, CloudNativePG operator (Helm), Kubernetes YAML, Go 1.24 + `pgx/v5`.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `infra/terraform/modules/compute/main.tf` | Modify | Add `worker2` VM + update dynamic group |
| `infra/terraform/modules/compute/outputs.tf` | Modify | Expose `worker2_public_ip`, `worker2_private_ip` |
| `infra/terraform/modules/storage/main.tf` | Modify | Add `postgres_backups` OCI bucket |
| `infra/terraform/modules/storage/outputs.tf` | Modify | Expose `postgres_backups_bucket_name` |
| `infra/terraform/main.tf` | Modify | Wire worker2 IP into DNS list + add backup bucket IAM policy |
| `infra/ansible/inventory.ini` | Modify | Add worker-2 host entry |
| `infra/ansible/roles/k8s-manifests/tasks/main.yml` | Modify | Add CNPG Helm install + postgres manifest deploy + Secret copy |
| `infra/k8s/postgres/namespace.yaml` | Create | `postgres` namespace |
| `infra/k8s/postgres/cluster.yaml` | Create | CloudNativePG `Cluster` CRD (3 instances) |
| `infra/k8s/postgres/scheduled-backup.yaml` | Create | `ScheduledBackup` CRD (daily 02:00 UTC) |
| `infra/k8s/backend/deployment.yaml` | Modify | Add `POSTGRES_URL` env from CNPG Secret |
| `backend/adapters/db/db.go` | Create | `pgxpool` init + migration runner |
| `backend/adapters/db/db_test.go` | Create | Unit test: error on empty POSTGRES_URL |
| `backend/adapters/db/migrations/001_users.sql` | Create | `users` table (pgcrypto + UUID) |
| `backend/main.go` | Modify | Conditional DB init if `POSTGRES_URL` set |
| `backend/go.mod` + `backend/go.sum` | Modify | Add `pgx/v5` dependency |

---

## Task 1: Add Worker-2 VM to Terraform Compute Module

**Files:**
- Modify: `infra/terraform/modules/compute/main.tf`
- Modify: `infra/terraform/modules/compute/outputs.tf`

- [ ] **Step 1: Add the worker2 instance resource**

Open `infra/terraform/modules/compute/main.tf`. After the closing `}` of `resource "oci_core_instance" "worker"`, add:

```hcl
resource "oci_core_instance" "worker2" {
  compartment_id      = var.compartment_ocid
  availability_domain = var.availability_domain
  display_name        = "zeropad-worker2"
  shape               = "VM.Standard.A1.Flex"

  shape_config {
    ocpus         = var.worker_vm_ocpus
    memory_in_gbs = var.worker_vm_memory_gbs
  }

  source_details {
    source_type             = "image"
    source_id               = var.image_id
    boot_volume_size_in_gbs = 50
  }

  create_vnic_details {
    subnet_id        = var.subnet_id
    assign_public_ip = true
    display_name     = "zeropad-worker2-vnic"
    hostname_label   = "zeropad-worker2"
  }

  metadata = {
    ssh_authorized_keys = "${var.ssh_public_key}\n${var.ssh_deploy_public_key}"
  }

  preserve_boot_volume = false
}
```

- [ ] **Step 2: Update the dynamic group matching rule to include worker2**

In the same file, replace the `matching_rule` in `resource "oci_identity_dynamic_group" "backend"`:

Old:
```hcl
  matching_rule  = "Any {instance.id = '${oci_core_instance.this.id}', instance.id = '${oci_core_instance.worker.id}'}"
```

New:
```hcl
  matching_rule  = "Any {instance.id = '${oci_core_instance.this.id}', instance.id = '${oci_core_instance.worker.id}', instance.id = '${oci_core_instance.worker2.id}'}"
```

- [ ] **Step 3: Add worker2 outputs**

Open `infra/terraform/modules/compute/outputs.tf`. Append at the end:

```hcl
output "worker2_public_ip" {
  description = "Public IP of the second k3s worker VM."
  value       = oci_core_instance.worker2.public_ip
}

output "worker2_private_ip" {
  description = "Private IP of the second k3s worker VM."
  value       = oci_core_instance.worker2.private_ip
}
```

- [ ] **Step 4: Commit**

```bash
git add infra/terraform/modules/compute/main.tf infra/terraform/modules/compute/outputs.tf
git commit -m "feat(infra): add worker2 VM to compute module for 3-node k3s cluster"
```

---

## Task 2: Add PostgreSQL Backup Bucket to Storage Module

**Files:**
- Modify: `infra/terraform/modules/storage/main.tf`
- Modify: `infra/terraform/modules/storage/outputs.tf`

- [ ] **Step 1: Add the backup bucket resource**

Open `infra/terraform/modules/storage/main.tf`. After the existing `oci_objectstorage_bucket.pads` resource, add:

```hcl
resource "oci_objectstorage_bucket" "postgres_backups" {
  compartment_id = var.compartment_ocid
  namespace      = var.object_storage_namespace
  name           = "zeropad-postgres-backups"
  access_type    = "NoPublicAccess"
  versioning     = "Disabled"
}
```

- [ ] **Step 2: Add the bucket name output**

Open `infra/terraform/modules/storage/outputs.tf`. Append:

```hcl
output "postgres_backups_bucket_name" {
  description = "Name of the OCI bucket used for PostgreSQL WAL archiving and base backups."
  value       = oci_objectstorage_bucket.postgres_backups.name
}
```

- [ ] **Step 3: Commit**

```bash
git add infra/terraform/modules/storage/main.tf infra/terraform/modules/storage/outputs.tf
git commit -m "feat(infra): add zeropad-postgres-backups OCI bucket for CNPG WAL archiving"
```

---

## Task 3: Wire Root Terraform — DNS and IAM

**Files:**
- Modify: `infra/terraform/main.tf`

- [ ] **Step 1: Add worker2 IP to the DNS worker list**

Open `infra/terraform/main.tf`. In the `module "dns"` block, change:

Old:
```hcl
  worker_public_ips  = [module.compute.worker_public_ip]
```

New:
```hcl
  worker_public_ips  = [module.compute.worker_public_ip, module.compute.worker2_public_ip]
```

- [ ] **Step 2: Add IAM policy for the postgres backup bucket**

In `infra/terraform/main.tf`, after the existing `oci_identity_policy.backend_object_storage` resource, add:

```hcl
resource "oci_identity_policy" "postgres_backup_object_storage" {
  compartment_id = var.compartment_ocid
  name           = "zeropad-postgres-backup-storage"
  description    = "Allow backend VMs to manage objects in the postgres backup bucket"
  statements = [
    "Allow dynamic-group zeropad-backend to manage objects in compartment id ${var.compartment_ocid} where target.bucket.name = 'zeropad-postgres-backups'",
  ]
}
```

- [ ] **Step 3: Commit**

```bash
git add infra/terraform/main.tf
git commit -m "feat(infra): wire worker2 DNS record and postgres backup bucket IAM policy"
```

---

## Task 4: Provision Infrastructure

> Prerequisites: OCI credentials configured (`~/.oci/config`). Run from the `infra/terraform/` directory.

- [ ] **Step 1: Preview the changes**

```bash
cd infra/terraform
terraform plan
```

Expected output includes:
- `+ oci_core_instance.worker2` (new worker VM)
- `+ oci_objectstorage_bucket.postgres_backups` (new bucket)
- `+ oci_identity_policy.postgres_backup_object_storage` (new policy)
- `~ oci_identity_dynamic_group.backend` (updated matching rule)
- `+ cloudflare_record.apex_workers["<worker2-ip>"]` (new DNS A record)

Verify no existing resources are destroyed. If Terraform shows destroy on `worker` or `this`, stop and investigate before proceeding.

- [ ] **Step 2: Apply**

```bash
terraform apply
```

Type `yes` when prompted.

- [ ] **Step 3: Capture worker2 public IP for Ansible**

```bash
terraform output worker2_public_ip
```

Copy this IP — you'll use it in Task 5.

---

## Task 5: Register Worker-2 with k3s

**Files:**
- Modify: `infra/ansible/inventory.ini`

- [ ] **Step 1: Add worker2 to the Ansible inventory**

Open `infra/ansible/inventory.ini`. Add a new line in the `[k3s_agent]` section:

```ini
[k3s_server]
zeropad-server ansible_host=129.213.95.227 ansible_user=opc ansible_ssh_private_key_file=~/.ssh/zeropad_deploy_rsa

[k3s_agent]
zeropad-worker  ansible_host=132.145.198.101 ansible_user=opc ansible_ssh_private_key_file=~/.ssh/zeropad_deploy_rsa
zeropad-worker2 ansible_host=<WORKER2_PUBLIC_IP> ansible_user=opc ansible_ssh_private_key_file=~/.ssh/zeropad_deploy_rsa

[k3s_cluster:children]
k3s_server
k3s_agent
```

Replace `<WORKER2_PUBLIC_IP>` with the IP from Task 4 Step 3.

- [ ] **Step 2: Run the k3s agent role on the new worker only**

```bash
cd infra/ansible
ansible-playbook playbook.yml --limit zeropad-worker2 --tags k3s
```

Expected: The k3s agent service is installed and started on worker2. The `creates:` guard in the role means it skips if already present on existing workers.

- [ ] **Step 3: Verify worker2 joined the cluster**

SSH into the server and check node status:

```bash
ssh -i ~/.ssh/zeropad_deploy_rsa opc@129.213.95.227 \
  "sudo k3s kubectl get nodes -o wide"
```

Expected output: 3 rows — `zeropad-server` (control-plane), `zeropad-worker`, `zeropad-worker2` — all with status `Ready`.

- [ ] **Step 4: Commit**

```bash
git add infra/ansible/inventory.ini
git commit -m "feat(infra): add worker2 to Ansible inventory for 3-node k3s cluster"
```

---

## Task 6: Create Kubernetes Postgres Manifests

**Files:**
- Create: `infra/k8s/postgres/namespace.yaml`
- Create: `infra/k8s/postgres/cluster.yaml`
- Create: `infra/k8s/postgres/scheduled-backup.yaml`

- [ ] **Step 1: Create the postgres namespace manifest**

Create `infra/k8s/postgres/namespace.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: postgres
```

- [ ] **Step 2: Create the CloudNativePG Cluster manifest**

Create `infra/k8s/postgres/cluster.yaml`:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: zeropad-pg
  namespace: postgres
spec:
  instances: 3
  imageName: ghcr.io/cloudnative-pg/postgresql:16

  postgresql:
    parameters:
      max_connections: "200"

  bootstrap:
    initdb:
      database: zeropad
      owner: zeropad

  storage:
    storageClass: local-path
    size: 20Gi

  affinity:
    podAntiAffinityType: required
    topologyKey: kubernetes.io/hostname

  backup:
    barmanObjectStore:
      destinationPath: "s3://zeropad-postgres-backups/"
      endpointURL: "https://REPLACE_OCI_NAMESPACE.compat.objectstorage.REPLACE_OCI_REGION.oraclecloud.com"
      s3Credentials:
        accessKeyId:
          name: postgres-backup-credentials
          key: ACCESS_KEY_ID
        secretAccessKey:
          name: postgres-backup-credentials
          key: SECRET_ACCESS_KEY
      wal:
        compression: gzip
    retentionPolicy: "7d"
```

**Fill in before applying:** Replace `REPLACE_OCI_NAMESPACE` with the value of `terraform output object_storage_namespace` and `REPLACE_OCI_REGION` with your OCI region (e.g. `us-ashburn-1`).

- [ ] **Step 3: Create the ScheduledBackup manifest**

Create `infra/k8s/postgres/scheduled-backup.yaml`:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: zeropad-pg-daily
  namespace: postgres
spec:
  schedule: "0 2 * * *"
  backupOwnerReference: self
  cluster:
    name: zeropad-pg
```

- [ ] **Step 4: Commit**

```bash
git add infra/k8s/postgres/
git commit -m "feat(k8s): add CloudNativePG Cluster and ScheduledBackup manifests"
```

---

## Task 7: Update Backend Deployment to Inject POSTGRES_URL

**Files:**
- Modify: `infra/k8s/backend/deployment.yaml`

- [ ] **Step 1: Add POSTGRES_URL env from the CNPG-generated Secret**

Open `infra/k8s/backend/deployment.yaml`. After the `envFrom` block (which pulls from `backend-env` ConfigMap), add an `env` section:

```yaml
          envFrom:
            - configMapRef:
                name: backend-env
          env:
            - name: POSTGRES_URL
              valueFrom:
                secretKeyRef:
                  name: zeropad-pg-app
                  key: uri
```

The full `containers` entry should now look like:

```yaml
      containers:
        - name: backend
          image: ghcr.io/victorfariafernandes/zeropad-backend:latest
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef:
                name: backend-env
          env:
            - name: POSTGRES_URL
              valueFrom:
                secretKeyRef:
                  name: zeropad-pg-app
                  key: uri
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

- [ ] **Step 2: Commit**

```bash
git add infra/k8s/backend/deployment.yaml
git commit -m "feat(k8s): inject POSTGRES_URL from CNPG app secret into backend pods"
```

---

## Task 8: Update Ansible k8s-manifests Role

**Files:**
- Modify: `infra/ansible/roles/k8s-manifests/tasks/main.yml`

- [ ] **Step 1: Add CNPG Helm install tasks and postgres manifest apply**

Open `infra/ansible/roles/k8s-manifests/tasks/main.yml`.

First, after the existing `Add ingress-nginx Helm repo` task and before the existing `Update Helm repos` task, insert one new task:

```yaml
- name: Add CloudNativePG Helm repo
  command: /usr/local/bin/helm repo add cloudnative-pg https://cloudnative-pg.github.io/charts --force-update
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml
```

Then, after the existing `Update Helm repos` task and before the existing `Install or upgrade ingress-nginx` task, insert:

```yaml
- name: Install or upgrade CloudNativePG operator
  command: >
    /usr/local/bin/helm upgrade --install cnpg cloudnative-pg/cloudnative-pg
    --namespace cnpg-system
    --create-namespace
    --version 1.25.0
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml

- name: Wait for CNPG operator to be ready
  command: >
    /usr/local/bin/k3s kubectl rollout status deployment/cnpg-cloudnative-pg
    -n cnpg-system --timeout=120s
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml
```

> **Version pin:** `1.25.0` is the current stable release as of this writing. Check https://github.com/cloudnative-pg/cloudnative-pg/releases for the latest and pin accordingly.

- [ ] **Step 2: Add backup credentials Secret creation task**

After the CNPG operator tasks, add (before the postgres namespace apply):

```yaml
- name: Create postgres backup credentials secret
  shell: >
    /usr/local/bin/k3s kubectl create secret generic postgres-backup-credentials
    --from-literal=ACCESS_KEY_ID={{ postgres_backup_access_key_id }}
    --from-literal=SECRET_ACCESS_KEY={{ postgres_backup_secret_access_key }}
    --namespace=postgres
    --dry-run=client -o yaml | /usr/local/bin/k3s kubectl apply -f -
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml
```

These are OCI Customer Secret Keys. Create them in the OCI Console under **Profile → Customer Secret Keys**. Pass them as `--extra-vars` when running the playbook.

- [ ] **Step 3: Add postgres manifest apply tasks**

Continue after the Secret creation task:

```yaml
- name: Apply postgres namespace
  command: /usr/local/bin/k3s kubectl apply -f /tmp/zeropad-k8s/postgres/namespace.yaml
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml

- name: Apply PostgreSQL cluster
  command: /usr/local/bin/k3s kubectl apply -f /tmp/zeropad-k8s/postgres/cluster.yaml
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml

- name: Apply scheduled backup
  command: /usr/local/bin/k3s kubectl apply -f /tmp/zeropad-k8s/postgres/scheduled-backup.yaml
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml

- name: Wait for PostgreSQL cluster to be ready
  command: >
    /usr/local/bin/k3s kubectl wait cluster/zeropad-pg
    --for=condition=Ready
    --namespace=postgres
    --timeout=300s
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml

- name: Copy CNPG app secret to zeropad namespace
  shell: >
    /usr/local/bin/k3s kubectl get secret zeropad-pg-app -n postgres -o yaml |
    sed 's/namespace: postgres/namespace: zeropad/' |
    sed '/resourceVersion:/d' |
    sed '/uid:/d' |
    sed '/creationTimestamp:/d' |
    /usr/local/bin/k3s kubectl apply -f -
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml
```

- [ ] **Step 4: Commit**

```bash
git add infra/ansible/roles/k8s-manifests/tasks/main.yml
git commit -m "feat(ansible): install CNPG operator, deploy postgres cluster, copy app secret"
```

---

## Task 9: Add pgx/v5 Dependency to Backend

**Files:**
- Modify: `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add the dependency**

```bash
cd backend
go get github.com/jackc/pgx/v5@latest
```

- [ ] **Step 2: Verify it was added**

```bash
grep pgx go.mod
```

Expected: `github.com/jackc/pgx/v5 v5.x.x`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat(backend): add pgx/v5 dependency for PostgreSQL"
```

---

## Task 10: Write Failing Test for DB Adapter

**Files:**
- Create: `backend/adapters/db/db_test.go`

- [ ] **Step 1: Write the test**

Create `backend/adapters/db/db_test.go`:

```go
package db_test

import (
	"context"
	"os"
	"testing"

	"zeropad-backend/adapters/db"
)

func TestInit_ErrorWhenNoURL(t *testing.T) {
	os.Unsetenv("POSTGRES_URL")
	_, err := db.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when POSTGRES_URL is not set, got nil")
	}
}
```

- [ ] **Step 2: Run the test — expect compile failure (package doesn't exist yet)**

```bash
cd backend
go test ./adapters/db/...
```

Expected: `cannot find package` or build error. This confirms the test is driving implementation.

---

## Task 11: Implement the DB Adapter

**Files:**
- Create: `backend/adapters/db/db.go`
- Create: `backend/adapters/db/migrations/001_users.sql`

- [ ] **Step 1: Create the migration SQL**

Create `backend/adapters/db/migrations/001_users.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: Create the DB adapter**

Create `backend/adapters/db/db.go`:

```go
package db

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_users.sql
var migration001 string

type DB struct {
	pool *pgxpool.Pool
}

func Init(ctx context.Context) (*DB, error) {
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		return nil, fmt.Errorf("POSTGRES_URL not set")
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	d := &DB{pool: pool}
	if err := d.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return d, nil
}

func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

func (d *DB) Close() {
	d.pool.Close()
}

func (d *DB) migrate(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, migration001)
	return err
}
```

- [ ] **Step 3: Run the test — expect it to pass**

```bash
cd backend
go test ./adapters/db/...
```

Expected:
```
ok  	zeropad-backend/adapters/db
```

- [ ] **Step 4: Run all backend tests to check for regressions**

```bash
cd backend
go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add backend/adapters/db/
git commit -m "feat(backend): add db adapter with pgxpool init and users migration"
```

---

## Task 12: Wire DB Init into main.go

**Files:**
- Modify: `backend/main.go`

- [ ] **Step 1: Update main.go to conditionally init the DB**

Open `backend/main.go`. Add the `context` and `db` imports, and add the DB init block at the start of `main()`:

```go
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
	if os.Getenv("POSTGRES_URL") != "" {
		database, err := db.Init(context.Background())
		if err != nil {
			log.Fatalf("failed to init database: %v", err)
		}
		defer database.Close()
		log.Printf("connected to PostgreSQL metadata store")
	}

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
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
cd backend
go build ./...
```

Expected: exits with code 0, no output.

- [ ] **Step 3: Run all tests**

```bash
cd backend
go test ./...
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add backend/main.go
git commit -m "feat(backend): init PostgreSQL metadata DB on startup when POSTGRES_URL is set"
```

---

## Task 13: Deploy Full Stack and Verify

> Prerequisites: OCI Customer Secret Keys created (see Task 8, Step 2 notes). Have the key ID and secret ready.
> Also fill in the OCI namespace and region placeholders in `infra/k8s/postgres/cluster.yaml` before this task.

- [ ] **Step 1: Run the full Ansible playbook**

```bash
cd infra/ansible
ansible-playbook playbook.yml \
  --extra-vars "release_tag=<your-tag> oci_namespace=<your-namespace> postgres_backup_access_key_id=<key-id> postgres_backup_secret_access_key=<secret>"
```

Replace the `<...>` placeholders with your actual values.

- [ ] **Step 2: Verify CNPG cluster is healthy**

SSH into the server:

```bash
ssh -i ~/.ssh/zeropad_deploy_rsa opc@129.213.95.227
sudo k3s kubectl get cluster -n postgres
```

Expected:
```
NAME         AGE   INSTANCES   READY   STATUS
zeropad-pg   2m    3           3       Cluster in healthy state
```

- [ ] **Step 3: Verify WAL archiving is active**

```bash
sudo k3s kubectl get cluster zeropad-pg -n postgres -o jsonpath='{.status.conditions}' | python3 -m json.tool
```

Look for `"continuousArchiving": "streaming"` or similar in the status.

- [ ] **Step 4: Verify the app secret was copied to zeropad namespace**

```bash
sudo k3s kubectl get secret zeropad-pg-app -n zeropad
```

Expected: Secret exists with `uri` key.

- [ ] **Step 5: Verify backend connected to Postgres**

```bash
sudo k3s kubectl logs -n zeropad deployment/backend --tail=20
```

Expected: contains `connected to PostgreSQL metadata store`.

- [ ] **Step 6: Verify users table was migrated**

```bash
sudo k3s kubectl exec -n postgres \
  $(sudo k3s kubectl get pods -n postgres -l cnpg.io/instanceRole=primary -o jsonpath='{.items[0].metadata.name}') \
  -- psql -U zeropad -d zeropad -c '\dt'
```

Expected: `users` table listed.

- [ ] **Step 7: Failover test**

```bash
# Delete the primary pod — CNPG should promote a standby
PRIMARY=$(sudo k3s kubectl get pods -n postgres -l cnpg.io/instanceRole=primary -o jsonpath='{.items[0].metadata.name}')
sudo k3s kubectl delete pod $PRIMARY -n postgres

# Wait ~30 seconds, then check cluster status
sleep 30
sudo k3s kubectl get cluster zeropad-pg -n postgres
```

Expected: Cluster returns to `healthy state` with 3/3 instances. A different pod is now primary.

- [ ] **Step 8: Run backend tests one final time**

```bash
cd backend
go test ./...
```

Expected: all pass.

- [ ] **Step 9: Final commit (CHANGELOG)**

Update `CHANGELOG.md` with an entry describing this deployment, then commit:

```bash
git add CHANGELOG.md
git commit -m "docs: update CHANGELOG for CloudNativePG metadata store deployment"
```
