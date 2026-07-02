"use client";

import { useState } from "react";
import { updateEmail, updateUsername, type User } from "@/app/_lib/auth";

export function ProfileSettingsPanel({
  user,
  onUserChange,
}: {
  user: User;
  onUserChange: (u: User) => void;
}) {
  return (
    <div className="flex flex-col gap-8 max-w-md">
      <UsernameField user={user} onUserChange={onUserChange} />
      <EmailField user={user} onUserChange={onUserChange} />
    </div>
  );
}

function UsernameField({
  user,
  onUserChange,
}: {
  user: User;
  onUserChange: (u: User) => void;
}) {
  const [draft, setDraft] = useState(user.username);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const dirty = draft.trim() !== "" && draft !== user.username;

  async function confirm(): Promise<void> {
    setError("");
    setLoading(true);
    try {
      const updated = await updateUsername(draft);
      onUserChange({ ...user, ...updated });
    } catch (err) {
      setError(err instanceof Error ? err.message : "update failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-zinc-500">Account name</label>
      <div className="flex gap-2">
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          className="h-10 px-3 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 font-mono text-sm outline-none focus:ring-2 focus:ring-black/20 dark:focus:ring-white/20 flex-1"
        />
        <button
          type="button"
          onClick={confirm}
          disabled={!dirty || loading}
          className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium disabled:opacity-50"
        >
          {loading ? "Saving…" : "Confirm change"}
        </button>
      </div>
      {error && <p className="text-sm text-red-500">{error}</p>}
    </div>
  );
}

function EmailField({
  user,
  onUserChange,
}: {
  user: User;
  onUserChange: (u: User) => void;
}) {
  const [draft, setDraft] = useState(user.email ?? "");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const dirty = draft !== (user.email ?? "");

  async function confirm(): Promise<void> {
    setError("");
    setLoading(true);
    try {
      const updated = await updateEmail(draft);
      onUserChange({ ...user, ...updated });
    } catch (err) {
      setError(err instanceof Error ? err.message : "update failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-zinc-500">Email</label>
      <div className="flex gap-2 items-center">
        <input
          type="email"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="you@example.com"
          className="h-10 px-3 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 font-mono text-sm outline-none focus:ring-2 focus:ring-black/20 dark:focus:ring-white/20 flex-1"
        />
        <button
          type="button"
          onClick={confirm}
          disabled={!dirty || loading}
          className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium disabled:opacity-50"
        >
          {loading ? "Saving…" : "Confirm change"}
        </button>
      </div>
      <span
        className={`text-xs w-fit px-2 py-0.5 rounded-full ${
          user.email_verified
            ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
            : "bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400"
        }`}
      >
        {user.email_verified ? "Verified" : "Not verified"}
      </span>
      {error && <p className="text-sm text-red-500">{error}</p>}
    </div>
  );
}
