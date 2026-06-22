"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  clearSession,
  loginPassword,
  loginWallet,
  passkeyLoginBegin,
  passkeyLoginFinish,
  passkeyRegisterBegin,
  passkeyRegisterFinish,
  saveSession,
  signup,
} from "@/app/_lib/auth";

type Tab = "signin" | "signup";
type SigninMethod = "password" | "wallet" | "passkey";
type SignupMethod = "password" | "wallet";

export function AuthPageClient() {
  const router = useRouter();
  const [tab, setTab] = useState<Tab>("signin");

  return (
    <div className="flex flex-col flex-1 items-center justify-center p-8">
      <main className="flex flex-col gap-6 w-full max-w-sm">
        <h1 className="text-4xl font-semibold tracking-tight text-center">
          zeropad
        </h1>

        <div className="flex rounded-lg border border-black/10 dark:border-white/15 overflow-hidden">
          <TabButton active={tab === "signin"} onClick={() => setTab("signin")}>
            Sign in
          </TabButton>
          <TabButton active={tab === "signup"} onClick={() => setTab("signup")}>
            Sign up
          </TabButton>
        </div>

        {tab === "signin" ? (
          <SignInForm onSuccess={() => router.push("/")} />
        ) : (
          <SignUpForm onSuccess={() => router.push("/")} />
        )}
      </main>
    </div>
  );
}

// ─── Sign In ─────────────────────────────────────────────────────────────────

function SignInForm({ onSuccess }: { onSuccess: () => void }) {
  const [method, setMethod] = useState<SigninMethod>("password");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      let token: string;

      if (method === "password") {
        ({ token } = await loginPassword(username, password));
      } else if (method === "wallet") {
        const { address, signature, message } = await signSIWELogin(username);
        ({ token } = await loginWallet(username, address, signature, message));
      } else {
        // passkey
        const options = await passkeyLoginBegin(username);
        const { startAuthentication } = await import("@simplewebauthn/browser");
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const credential = await startAuthentication({ optionsJSON: options as any });
        ({ token } = await passkeyLoginFinish(username, credential));
      }

      saveSession(token);
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : "sign in failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <Input
        label="Username"
        value={username}
        onChange={setUsername}
        placeholder="alice"
        required
      />

      <MethodPicker
        options={[
          { value: "password", label: "Password" },
          { value: "wallet", label: "Wallet" },
          { value: "passkey", label: "Passkey" },
        ]}
        value={method}
        onChange={(v) => setMethod(v as SigninMethod)}
      />

      {method === "password" && (
        <Input
          label="Password"
          type="password"
          value={password}
          onChange={setPassword}
          placeholder="••••••••"
          required
        />
      )}

      {error && <p className="text-sm text-red-500">{error}</p>}

      <button
        type="submit"
        disabled={loading}
        className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium disabled:opacity-50"
      >
        {loading ? "Signing in…" : "Sign in"}
      </button>
    </form>
  );
}

// ─── Sign Up ─────────────────────────────────────────────────────────────────

function SignUpForm({ onSuccess }: { onSuccess: () => void }) {
  const [method, setMethod] = useState<SignupMethod>("password");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [offerPasskey, setOfferPasskey] = useState(false);
  const [savedToken, setSavedToken] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      let result: { token: string };

      if (method === "password") {
        if (password.length < 8) {
          setError("Password must be at least 8 characters");
          return;
        }
        result = await signup({
          username,
          email: email || undefined,
          method: "password",
          password,
        });
      } else {
        const { address, signature, message } = await signSIWESignup(username);
        result = await signup({
          username,
          email: email || undefined,
          method: "siwe",
          wallet_address: address,
          siwe_signature: signature,
          siwe_message: message,
        });
      }

      saveSession(result.token);
      setSavedToken(result.token);
      setOfferPasskey(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "sign up failed");
    } finally {
      setLoading(false);
    }
  }

  async function addPasskey() {
    setLoading(true);
    try {
      const options = await passkeyRegisterBegin();
      const { startRegistration } = await import("@simplewebauthn/browser");
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const credential = await startRegistration({ optionsJSON: options as any });
      await passkeyRegisterFinish(credential);
    } catch {
      // passkey is optional — ignore errors
    } finally {
      setLoading(false);
      onSuccess();
    }
  }

  if (offerPasskey) {
    return (
      <div className="flex flex-col gap-4">
        <p className="text-sm text-zinc-500 text-center">
          Account created! Add a passkey for faster sign-in next time?
        </p>
        <button
          onClick={addPasskey}
          disabled={loading}
          className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium disabled:opacity-50"
        >
          {loading ? "Registering passkey…" : "Add passkey"}
        </button>
        <button
          onClick={onSuccess}
          className="text-sm text-zinc-500 underline text-center"
        >
          Skip
        </button>
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <Input
        label="Username"
        value={username}
        onChange={setUsername}
        placeholder="alice"
        required
      />
      <Input
        label="Email (optional)"
        type="email"
        value={email}
        onChange={setEmail}
        placeholder="alice@example.com"
      />

      <MethodPicker
        options={[
          { value: "password", label: "Password" },
          { value: "wallet", label: "Wallet" },
        ]}
        value={method}
        onChange={(v) => setMethod(v as SignupMethod)}
      />

      {method === "password" && (
        <Input
          label="Password"
          type="password"
          value={password}
          onChange={setPassword}
          placeholder="min 8 characters"
          required
        />
      )}

      {error && <p className="text-sm text-red-500">{error}</p>}

      <button
        type="submit"
        disabled={loading}
        className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium disabled:opacity-50"
      >
        {loading ? "Creating account…" : "Create account"}
      </button>
    </form>
  );
}

// ─── Shared UI components ─────────────────────────────────────────────────────

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex-1 h-9 text-sm font-medium transition-colors ${
        active
          ? "bg-foreground text-background"
          : "text-zinc-500 hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}

function Input({
  label,
  value,
  onChange,
  placeholder,
  type = "text",
  required,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  type?: string;
  required?: boolean;
}) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-zinc-500">{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        required={required}
        className="h-10 px-3 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 font-mono text-sm outline-none focus:ring-2 focus:ring-black/20 dark:focus:ring-white/20"
      />
    </div>
  );
}

function MethodPicker({
  options,
  value,
  onChange,
}: {
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-zinc-500">Method</label>
      <div className="flex rounded-lg border border-black/10 dark:border-white/15 overflow-hidden">
        {options.map((opt) => (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            className={`flex-1 h-9 text-sm font-medium transition-colors ${
              value === opt.value
                ? "bg-foreground text-background"
                : "text-zinc-500 hover:text-foreground"
            }`}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </div>
  );
}

// ─── SIWE helpers ─────────────────────────────────────────────────────────────

async function signSIWELogin(
  username: string,
): Promise<{ address: string; signature: string; message: string }> {
  const { ethers } = await import("ethers");
  if (!window.ethereum) throw new Error("No browser wallet found. Install MetaMask.");
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const provider = new ethers.BrowserProvider(window.ethereum as any);
  const signer = await provider.getSigner();
  const address = await signer.getAddress();
  const message = `zeropad login: ${username}\nWallet: ${address}`;
  const signature = await signer.signMessage(message);
  return { address, signature, message };
}

async function signSIWESignup(
  username: string,
): Promise<{ address: string; signature: string; message: string }> {
  return signSIWELogin(username);
}
