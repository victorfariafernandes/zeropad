import { apiFetch } from "@/app/_lib/api";

// ─── Session storage ──────────────────────────────────────────────────────────

export function getSession(): string | null {
  return sessionStorage.getItem("session_token");
}

export function saveSession(token: string): void {
  sessionStorage.setItem("session_token", token);
}

export function clearSession(): void {
  sessionStorage.removeItem("session_token");
}

function authHeaders(): HeadersInit {
  const token = getSession();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

// ─── Types ───────────────────────────────────────────────────────────────────

export interface User {
  id: string;
  username: string;
  email?: string;
  wallet_address?: string;
  has_passkey: boolean;
}

export interface SignupData {
  username: string;
  email?: string;
  method: "password" | "siwe";
  password?: string;
  wallet_address?: string;
  siwe_signature?: string;
  siwe_message?: string;
}

// ─── API calls ───────────────────────────────────────────────────────────────

export async function signup(data: SignupData): Promise<{ token: string }> {
  const res = await apiFetch("/auth/signup", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `signup failed: ${res.status}`);
  }
  return res.json();
}

export async function loginPassword(
  username: string,
  password: string,
): Promise<{ token: string }> {
  const res = await apiFetch("/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, method: "password", password }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `login failed: ${res.status}`);
  }
  return res.json();
}

export async function loginWallet(
  username: string,
  address: string,
  signature: string,
  message: string,
): Promise<{ token: string }> {
  const res = await apiFetch("/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      username,
      method: "siwe",
      wallet_address: address,
      siwe_signature: signature,
      siwe_message: message,
    }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `login failed: ${res.status}`);
  }
  return res.json();
}

export async function getMe(): Promise<User | null> {
  const token = getSession();
  if (!token) return null;
  const res = await apiFetch("/auth/me", { headers: authHeaders() });
  if (!res.ok) return null;
  return res.json();
}

export async function passkeyRegisterBegin(): Promise<unknown> {
  const res = await apiFetch("/auth/passkey/register/begin", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
  });
  if (!res.ok) throw new Error("passkey registration unavailable");
  return res.json();
}

export async function passkeyRegisterFinish(credential: unknown): Promise<void> {
  const res = await apiFetch("/auth/passkey/register/finish", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(credential),
  });
  if (!res.ok) throw new Error("passkey registration failed");
}

export async function passkeyLoginBegin(
  username: string,
): Promise<unknown> {
  const res = await apiFetch("/auth/passkey/login/begin", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? "passkey login unavailable");
  }
  return res.json();
}

export async function passkeyLoginFinish(
  username: string,
  credential: unknown,
): Promise<{ token: string }> {
  const res = await apiFetch("/auth/passkey/login/finish", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, credential }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? "passkey login failed");
  }
  return res.json();
}
