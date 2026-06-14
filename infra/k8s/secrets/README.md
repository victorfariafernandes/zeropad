# TLS Secret Bootstrap

The TLS secret `zeropad-origin-tls` is NOT committed to git. It must be created once on the cluster using the Cloudflare origin certificates stored in `infra/ansible/files/`.

Run this once after the k3s cluster is up and the namespace exists:

```bash
kubectl create secret tls zeropad-origin-tls \
  --cert=infra/ansible/files/origin.pem \
  --key=infra/ansible/files/origin.key \
  -n zeropad
```

To renew: delete the secret and re-run the command above with updated cert files.
