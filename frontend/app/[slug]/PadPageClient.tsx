"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { PadEditor } from "./PadEditor";

export function PadPageClient() {
  const router = useRouter();
  const [slug, setSlug] = useState<string>("_");

  useEffect(() => {
    const raw = window.location.pathname.slice(1) || "_";
    if (raw.startsWith("_")) {
      router.replace("/");
      return;
    }
    setSlug(raw);
  }, [router]);

  return <PadEditor slug={slug} />;
}
