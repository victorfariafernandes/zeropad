"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

export default function Home() {
  const router = useRouter();
  const [slug, setSlug] = useState("");

  function go() {
    const clean = slug.trim().replace(/^\/+/, "");
    if (!clean) return;
    if (clean.startsWith("_")) return;
    router.push(`/${clean}`);
  }

  return (
    <div className="flex flex-col flex-1 items-center justify-center p-8">
      <main className="flex flex-col items-center gap-6 w-full max-w-md">
        <h1 className="text-4xl font-semibold tracking-tight">zeropad</h1>
        <p className="text-sm text-zinc-500">any url is a pad</p>
        <div className="flex gap-2 w-full">
          <input
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && go()}
            placeholder="page-name"
            className="flex-1 h-10 px-3 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 font-mono text-sm outline-none focus:ring-2 focus:ring-black/20 dark:focus:ring-white/20"
          />
          <button
            onClick={go}
            className="h-10 px-4 rounded-lg bg-foreground text-background text-sm font-medium"
          >
            Go
          </button>
        </div>
      </main>
    </div>
  );
}
