# TLS Secret Bootstrap

The TLS secret `zeropad-origin-tls` is NOT committed to git. It must be created once on the cluster using the Cloudflare origin certificates stored in `infra/ansible/files/`.

Run this once after the k3s cluster is up and the namespace exists:

```bash
k3s kubectl create secret tls zeropad-origin-tls \
  --cert=infra/ansible/files/origin.pem \
  --key=infra/ansible/files/origin.key \
  -n zeropad
```

To renew: delete the secret and re-run the command above with updated cert files.

## Required GitHub Actions Secrets

The following secrets must be set in the GitHub repository's `production` environment before the deploy workflow can run:

| Secret | Description | How to get it |
|--------|-------------|---------------|
| `SSH_PRIVATE_KEY` | Private key for SSH access to both VMs | The deploy SSH key pair |
| `SSH_KNOWN_HOST` | Host fingerprint for Node 1 (server) | `ssh-keyscan <node1-public-ip>` |
| `WORKER_SSH_KNOWN_HOST` | Host fingerprint for Node 2 (worker) | `ssh-keyscan $(terraform -chdir=infra/terraform output -raw worker_public_ip)` |
| `OCI_NAMESPACE` | OCI Object Storage namespace | `terraform -chdir=infra/terraform output -raw object_storage_namespace` |

**Obsolete secrets** (can be removed from GitHub settings):
- `VM_IP` — was used for inline inventory; Node 1's IP is now committed in `inventory.ini`
