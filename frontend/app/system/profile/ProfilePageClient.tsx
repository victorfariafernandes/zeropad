"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { getMe, type User } from "@/app/_lib/auth";

type Section = "profile" | "api" | "groups" | "billing";

const NAV: { id: Section; label: string }[] = [
  { id: "profile", label: "Profile settings" },
  { id: "api", label: "API" },
  { id: "groups", label: "Groups" },
  { id: "billing", label: "Billing" },
];

export function ProfilePageClient() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [active, setActive] = useState<Section>("profile");

  useEffect(() => {
    getMe().then((u) => {
      if (!u) { router.push("/_/login"); return; }
      setUser(u);
    });
  }, [router]);

  if (!user) return null;

  const initials = user.username.slice(0, 2).toUpperCase();

  return (
    <div className="flex flex-row flex-1 min-h-screen">
      <aside className="w-56 border-r border-black/10 dark:border-white/15 p-6 flex flex-col gap-6">
        <div className="flex flex-col items-start gap-2">
          <div className="w-16 h-16 rounded-full bg-foreground text-background text-xl font-semibold flex items-center justify-center select-none">
            {initials}
          </div>
          <span className="text-sm font-medium">{user.username}</span>
        </div>

        <hr className="border-black/10 dark:border-white/15" />

        <nav className="flex flex-col gap-1">
          {NAV.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => setActive(id)}
              className={`w-full text-left px-3 py-2 rounded-lg text-sm transition-colors ${
                active === id
                  ? "bg-foreground text-background"
                  : "hover:bg-zinc-100 dark:hover:bg-zinc-900"
              }`}
            >
              {label}
            </button>
          ))}
        </nav>
      </aside>

      <main className="flex-1 p-8" />
    </div>
  );
}
