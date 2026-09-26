"use client";

import { useEffect } from "react";

// Registers /sw.js so browsers offer "Install app" / Add to Home Screen.
// Skipped in dev to avoid a stale worker fighting HMR.
export function ServiceWorkerRegister() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production" || !("serviceWorker" in navigator)) return;
    const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
    navigator.serviceWorker
      .register(`${base}/sw.js`, { scope: `${base}/`, updateViaCache: "none" })
      .catch((err) => console.warn("Service worker registration failed:", err));
  }, []);
  return null;
}
