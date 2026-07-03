"use client";

import { useEffect, useState } from "react";
import {
  type ApiKey,
  type Role,
  attachRole,
  createApiKey,
  createRole,
  deleteRole,
  detachRole,
  listApiKeys,
  listAttachedRoles,
  listRoles,
  revokeApiKey,
} from "@/app/_lib/apiKeys";

export function ApiKeysPanel() {
  const [keys, setKeys] = useState<ApiKey[] | null>(null);
  const [roles, setRoles] = useState<Role[] | null>(null);
  const [revealedKey, setRevealedKey] = useState<ApiKey | null>(null);
  const [error, setError] = useState("");

  async function refresh(): Promise<void> {
    try {
      const [k, r] = await Promise.all([listApiKeys(), listRoles()]);
      setKeys(k);
      setRoles(r);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load");
    }
  }

  useEffect(() => {
    listApiKeys().then(setKeys).catch((err) => setError(err instanceof Error ? err.message : "failed to load"));
    listRoles().then(setRoles).catch((err) => setError(err instanceof Error ? err.message : "failed to load"));
  }, []);

  if (!keys || !roles) return null;

  return (
    <div className="flex flex-col gap-10 max-w-2xl">
      {revealedKey && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border border-amber-400/50 bg-amber-50 dark:bg-amber-950/30">
          <p className="text-sm font-medium">
            Copy this key now — it won&apos;t be shown again.
          </p>
          <div className="flex gap-2">
            <code className="flex-1 px-3 py-2 rounded-lg bg-white dark:bg-zinc-950 border border-black/10 dark:border-white/15 font-mono text-xs break-all">
              {revealedKey.key}
            </code>
            <button
              type="button"
              onClick={() => navigator.clipboard.writeText(revealedKey.key ?? "")}
              className="h-fit px-3 py-2 rounded-lg bg-foreground text-background text-xs font-medium"
            >
              Copy
            </button>
          </div>
          <button
            type="button"
            onClick={() => setRevealedKey(null)}
            className="text-xs text-zinc-500 w-fit hover:underline"
          >
            Dismiss
          </button>
        </div>
      )}

      {error && <p className="text-sm text-red-500">{error}</p>}

      <KeysSection
        keys={keys}
        roles={roles}
        onCreated={(k) => {
          setRevealedKey(k);
          refresh();
        }}
        onChanged={refresh}
      />
      <RolesSection roles={roles} onChanged={refresh} />
    </div>
  );
}

function KeysSection({
  keys,
  roles,
  onCreated,
  onChanged,
}: {
  keys: ApiKey[];
  roles: Role[];
  onCreated: (k: ApiKey) => void;
  onChanged: () => void;
}) {
  const [name, setName] = useState("");
  const [restricted, setRestricted] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function create(): Promise<void> {
    setError("");
    setLoading(true);
    try {
      const key = await createApiKey(name, restricted);
      setName("");
      setRestricted(false);
      onCreated(key);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to create key");
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold">API keys</h2>

      <div className="flex flex-col gap-2">
        {keys.length === 0 && <p className="text-sm text-zinc-500">No API keys yet.</p>}
        {keys.map((k) => (
          <KeyRow key={k.id} apiKey={k} roles={roles} onChanged={onChanged} />
        ))}
      </div>

      <div className="flex gap-2 items-center">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Key name"
          className="h-10 px-3 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 font-mono text-sm outline-none focus:ring-2 focus:ring-black/20 dark:focus:ring-white/20 flex-1"
        />
        <label className="flex items-center gap-1 text-xs text-zinc-500">
          <input type="checkbox" checked={restricted} onChange={(e) => setRestricted(e.target.checked)} />
          Restricted
        </label>
        <button
          type="button"
          onClick={create}
          disabled={!name.trim() || loading}
          className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium disabled:opacity-50"
        >
          {loading ? "Creating…" : "Create key"}
        </button>
      </div>
      {error && <p className="text-sm text-red-500">{error}</p>}
    </section>
  );
}

function KeyRow({
  apiKey,
  roles,
  onChanged,
}: {
  apiKey: ApiKey;
  roles: Role[];
  onChanged: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [error, setError] = useState("");

  async function revoke(): Promise<void> {
    try {
      await revokeApiKey(apiKey.id);
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to revoke");
    }
  }

  return (
    <div data-cy="api-key-row" className="flex flex-col gap-2 p-3 rounded-lg border border-black/10 dark:border-white/15">
      <div className="flex items-center justify-between gap-2">
        <div className="flex flex-col">
          <span className="text-sm font-medium">{apiKey.name}</span>
          <span className="text-xs text-zinc-500">
            {new Date(apiKey.created_at).toLocaleDateString()}
            {apiKey.restricted && " · restricted"}
            {apiKey.revoked_at && " · revoked"}
          </span>
        </div>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            className="text-xs px-3 py-1.5 rounded-lg border border-black/10 dark:border-white/15"
          >
            Roles
          </button>
          {!apiKey.revoked_at && (
            <button
              type="button"
              onClick={revoke}
              className="text-xs px-3 py-1.5 rounded-lg border border-red-400/50 text-red-500"
            >
              Revoke
            </button>
          )}
        </div>
      </div>
      {expanded && <KeyRoleAssignment apiKeyId={apiKey.id} roles={roles} />}
      {error && <p className="text-xs text-red-500">{error}</p>}
    </div>
  );
}

function KeyRoleAssignment({ apiKeyId, roles }: { apiKeyId: string; roles: Role[] }) {
  const [assigned, setAssigned] = useState<Set<string> | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    listAttachedRoles(apiKeyId)
      .then((ids) => setAssigned(new Set(ids)))
      .catch((err) => setError(err instanceof Error ? err.message : "failed to load roles"));
  }, [apiKeyId]);

  async function toggle(current: Set<string>, roleId: string): Promise<void> {
    setError("");
    try {
      const next = new Set(current);
      if (next.has(roleId)) {
        await detachRole(apiKeyId, roleId);
        next.delete(roleId);
      } else {
        await attachRole(apiKeyId, roleId);
        next.add(roleId);
      }
      setAssigned(next);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to update role");
    }
  }

  if (!assigned) return null;
  const roleIds = assigned;

  if (roles.length === 0) {
    return <p className="text-xs text-zinc-500">No roles defined yet.</p>;
  }

  return (
    <div className="flex flex-col gap-1 pl-1">
      {roles.map((role) => (
        <label key={role.id} className="flex items-center gap-2 text-xs">
          <input
            type="checkbox"
            checked={roleIds.has(role.id)}
            onChange={() => toggle(roleIds, role.id)}
          />
          {role.name}
        </label>
      ))}
      {error && <p className="text-xs text-red-500">{error}</p>}
    </div>
  );
}

function RolesSection({ roles, onChanged }: { roles: Role[]; onChanged: () => void }) {
  const [name, setName] = useState("");
  const [canRead, setCanRead] = useState(true);
  const [canWrite, setCanWrite] = useState(false);
  const [canDelete, setCanDelete] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function create(): Promise<void> {
    setError("");
    setLoading(true);
    try {
      await createRole(name, canRead, canWrite, canDelete);
      setName("");
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to create role");
    } finally {
      setLoading(false);
    }
  }

  async function remove(id: string): Promise<void> {
    try {
      await deleteRole(id);
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to delete role");
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold">Roles</h2>

      <div className="flex flex-col gap-2">
        {roles.length === 0 && <p className="text-sm text-zinc-500">No roles yet.</p>}
        {roles.map((role) => (
          <div
            key={role.id}
            className="flex items-center justify-between gap-2 p-3 rounded-lg border border-black/10 dark:border-white/15"
          >
            <div className="flex flex-col">
              <span className="text-sm font-medium">{role.name}</span>
              <span className="text-xs text-zinc-500">
                {[role.can_read && "read", role.can_write && "write", role.can_delete && "delete"]
                  .filter(Boolean)
                  .join(", ")}
              </span>
            </div>
            <button
              type="button"
              onClick={() => remove(role.id)}
              className="text-xs px-3 py-1.5 rounded-lg border border-red-400/50 text-red-500"
            >
              Delete
            </button>
          </div>
        ))}
      </div>

      <div className="flex gap-2 items-center flex-wrap">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Role name"
          className="h-10 px-3 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 font-mono text-sm outline-none focus:ring-2 focus:ring-black/20 dark:focus:ring-white/20 flex-1 min-w-[10rem]"
        />
        <label className="flex items-center gap-1 text-xs text-zinc-500">
          <input type="checkbox" checked={canRead} onChange={(e) => setCanRead(e.target.checked)} />
          Read
        </label>
        <label className="flex items-center gap-1 text-xs text-zinc-500">
          <input type="checkbox" checked={canWrite} onChange={(e) => setCanWrite(e.target.checked)} />
          Write
        </label>
        <label className="flex items-center gap-1 text-xs text-zinc-500">
          <input type="checkbox" checked={canDelete} onChange={(e) => setCanDelete(e.target.checked)} />
          Delete
        </label>
        <button
          type="button"
          onClick={create}
          disabled={!name.trim() || loading}
          className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium disabled:opacity-50"
        >
          {loading ? "Creating…" : "Create role"}
        </button>
      </div>
      {error && <p className="text-sm text-red-500">{error}</p>}
    </section>
  );
}
