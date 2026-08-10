"use client";

import { useEffect, useState } from "react";

const ONE_HOUR = 60 * 60 * 1000;
const SIX_HOURS = 6 * ONE_HOUR;

function pad2(n: number): string {
  return n.toString().padStart(2, "0");
}

function formatRemaining(remainingMs: number): string {
  const totalSeconds = Math.floor(remainingMs / 1000);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (days >= 1) return `${days}d ${pad2(hours)}h ${pad2(minutes)}m ${pad2(seconds)}s`;
  if (hours >= 1) return `${hours}h ${pad2(minutes)}m ${pad2(seconds)}s`;
  if (minutes >= 1) return `${minutes}m ${pad2(seconds)}s`;
  return `${seconds}s`;
}

type PadTimerProps = { expiresAt: string | undefined };

export function PadTimer({ expiresAt }: PadTimerProps): React.ReactElement | null {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!expiresAt) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [expiresAt]);

  if (!expiresAt) return null;

  const remainingMs = new Date(expiresAt).getTime() - now;

  if (remainingMs <= 0) {
    return <span className="font-mono text-xs text-red-500">expired</span>;
  }

  const colorClass =
    remainingMs < ONE_HOUR
      ? "text-red-500"
      : remainingMs < SIX_HOURS
        ? "text-amber-500"
        : "text-zinc-400";

  return (
    <span className={`font-mono text-xs ${colorClass}`}>{formatRemaining(remainingMs)}</span>
  );
}
