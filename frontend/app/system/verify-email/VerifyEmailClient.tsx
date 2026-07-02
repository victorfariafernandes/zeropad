"use client";

import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { verifyEmail } from "@/app/_lib/auth";

type Status = "verifying" | "success" | "error";

export function VerifyEmailClient() {
  const searchParams = useSearchParams();
  const token = searchParams.get("token");
  const [status, setStatus] = useState<Status>(token ? "verifying" : "error");
  const [error, setError] = useState(token ? "" : "Missing verification token");

  useEffect(() => {
    if (!token) return;
    verifyEmail(token)
      .then(() => setStatus("success"))
      .catch((err) => {
        setStatus("error");
        setError(err instanceof Error ? err.message : "verification failed");
      });
  }, [token]);

  return (
    <div className="flex flex-col flex-1 items-center justify-center p-8">
      <div className="flex flex-col gap-4 items-center max-w-sm text-center">
        {status === "verifying" && (
          <p className="text-sm text-zinc-500">Verifying your email…</p>
        )}
        {status === "success" && (
          <p className="text-sm text-green-600">Your email has been verified.</p>
        )}
        {status === "error" && <p className="text-sm text-red-500">{error}</p>}
      </div>
    </div>
  );
}
