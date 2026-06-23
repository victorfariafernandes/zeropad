"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { getMe, logout, type User } from "@/app/_lib/auth";

export function UserBubble() {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<User | null | undefined>(undefined);
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    getMe().then(setUser);
  }, []);

  useEffect(() => {
    if (!open) return;
    function onMouseDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onMouseDown);
    return () => document.removeEventListener("mousedown", onMouseDown);
  }, [open]);

  if (pathname === "/_/login" || user === undefined) return null;

  if (!user) {
    return (
      <div className="fixed top-4 right-4 z-50">
        <a
          href="/_/login"
          className="text-sm text-zinc-500 hover:text-foreground transition-colors"
        >
          Sign in
        </a>
      </div>
    );
  }

  const initials = user.username.slice(0, 2).toUpperCase();

  async function handleLogout() {
    await logout();
    router.push("/_/login");
  }

  return (
    <div ref={ref} className="fixed top-4 right-4 z-50">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-8 h-8 rounded-full bg-foreground text-background text-xs font-semibold flex items-center justify-center cursor-pointer select-none"
        aria-label="Account menu"
      >
        {initials}
      </button>

      {open && (
        <div className="absolute right-0 top-10 w-36 rounded-lg border border-black/10 dark:border-white/15 bg-white dark:bg-zinc-950 shadow-md overflow-hidden">
          <button
            onClick={() => { setOpen(false); router.push("/_/profile"); }}
            className="w-full px-4 py-2 text-sm text-left hover:bg-zinc-100 dark:hover:bg-zinc-900 transition-colors"
          >
            Profile
          </button>
          <button
            onClick={handleLogout}
            className="w-full px-4 py-2 text-sm text-left hover:bg-zinc-100 dark:hover:bg-zinc-900 transition-colors"
          >
            Logout
          </button>
        </div>
      )}
    </div>
  );
}
