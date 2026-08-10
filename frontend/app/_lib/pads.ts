import { apiFetch } from "./api";
import type { DeriverId } from "./crypto";

export type PadData = {
  content: string;
  encrypted: boolean;
  verifyBlob: string;
  deriverId: DeriverId | "";
  writeToken?: string;
  newWriteToken?: string;
  expiresAt?: string;
};

export async function getPad(slug: string): Promise<PadData | null> {
  const res = await apiFetch(`/pads/${slug}`);
  if (res.status === 404) return null;
  if (!res.ok) throw new Error("failed to get pad");
  const body = (await res.json()) as {
    content?: string;
    encrypted: boolean;
    verify_blob?: string;
    deriver_id: string;
    expires_at?: string;
  };
  return {
    content: body.content ?? "",
    encrypted: body.encrypted,
    verifyBlob: body.verify_blob ?? "",
    deriverId: (body.deriver_id ?? "") as DeriverId | "",
    expiresAt: body.expires_at,
  };
}

export async function getPadContent(slug: string, writeToken: string): Promise<PadData> {
  const res = await apiFetch(`/pads/${slug}`, {
    headers: { "X-Write-Token": writeToken },
  });
  if (res.status === 403) throw new Error("write token invalid");
  if (!res.ok) throw new Error("failed to get pad content");
  const body = (await res.json()) as {
    content: string;
    encrypted: boolean;
    verify_blob: string;
    deriver_id: string;
    expires_at?: string;
  };
  return {
    content: body.content,
    encrypted: body.encrypted,
    verifyBlob: body.verify_blob,
    deriverId: (body.deriver_id ?? "") as DeriverId | "",
    expiresAt: body.expires_at,
  };
}

export async function setPad(slug: string, data: PadData): Promise<{ expiresAt?: string }> {
  const res = await apiFetch(`/pads/${slug}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      content: data.content,
      encrypted: data.encrypted,
      verify_blob: data.verifyBlob,
      deriver_id: data.deriverId,
      write_token: data.writeToken ?? "",
      new_write_token: data.newWriteToken ?? "",
    }),
  });
  if (res.status === 429) throw new Error("rate limit exceeded");
  if (res.status === 403) throw new Error("write token invalid");
  if (!res.ok) throw new Error("failed to save pad");
  const body = (await res.json()) as { expires_at?: string };
  return { expiresAt: body.expires_at };
}
