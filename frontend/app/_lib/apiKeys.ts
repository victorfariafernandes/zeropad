import { apiFetch } from "@/app/_lib/api";
import { getSession } from "@/app/_lib/auth";

function authHeaders(): HeadersInit {
  const token = getSession();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function unwrap<T>(res: Response, fallback: string): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? fallback);
  }
  return res.json();
}

// ─── Types ───────────────────────────────────────────────────────────────────

export interface ApiKey {
  id: string;
  name: string;
  restricted: boolean;
  created_at: string;
  revoked_at?: string;
  key?: string; // only present once, in the create response
}

export interface Role {
  id: string;
  name: string;
  can_read: boolean;
  can_write: boolean;
  can_delete: boolean;
  created_at: string;
}

// ─── API keys ────────────────────────────────────────────────────────────────

export async function listApiKeys(): Promise<ApiKey[]> {
  const res = await apiFetch("/api-keys", { headers: authHeaders() });
  return unwrap(res, "failed to list api keys");
}

export async function createApiKey(name: string, restricted: boolean): Promise<ApiKey> {
  const res = await apiFetch("/api-keys", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name, restricted }),
  });
  return unwrap(res, "failed to create api key");
}

export async function updateApiKey(id: string, name: string, restricted: boolean): Promise<void> {
  const res = await apiFetch(`/api-keys/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name, restricted }),
  });
  await unwrap(res, "failed to update api key");
}

export async function revokeApiKey(id: string): Promise<void> {
  const res = await apiFetch(`/api-keys/${id}`, { method: "DELETE", headers: authHeaders() });
  await unwrap(res, "failed to revoke api key");
}

export async function listAttachedRoles(apiKeyId: string): Promise<string[]> {
  const res = await apiFetch(`/api-keys/${apiKeyId}/roles`, { headers: authHeaders() });
  return unwrap(res, "failed to list attached roles");
}

export async function attachRole(apiKeyId: string, roleId: string): Promise<void> {
  const res = await apiFetch(`/api-keys/${apiKeyId}/roles`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ role_id: roleId }),
  });
  await unwrap(res, "failed to attach role");
}

export async function detachRole(apiKeyId: string, roleId: string): Promise<void> {
  const res = await apiFetch(`/api-keys/${apiKeyId}/roles/${roleId}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  await unwrap(res, "failed to detach role");
}

// ─── Roles ───────────────────────────────────────────────────────────────────

export async function listRoles(): Promise<Role[]> {
  const res = await apiFetch("/roles", { headers: authHeaders() });
  return unwrap(res, "failed to list roles");
}

export async function createRole(
  name: string,
  canRead: boolean,
  canWrite: boolean,
  canDelete: boolean,
): Promise<Role> {
  const res = await apiFetch("/roles", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name, can_read: canRead, can_write: canWrite, can_delete: canDelete }),
  });
  return unwrap(res, "failed to create role");
}

export async function updateRole(
  id: string,
  name: string,
  canRead: boolean,
  canWrite: boolean,
  canDelete: boolean,
): Promise<void> {
  const res = await apiFetch(`/roles/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name, can_read: canRead, can_write: canWrite, can_delete: canDelete }),
  });
  await unwrap(res, "failed to update role");
}

export async function deleteRole(id: string): Promise<void> {
  const res = await apiFetch(`/roles/${id}`, { method: "DELETE", headers: authHeaders() });
  await unwrap(res, "failed to delete role");
}
