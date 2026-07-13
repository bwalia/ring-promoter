"use client";

import { useEffect } from "react";
import { ScrollTrigger } from "./setup";

// ScrollTrigger measures trigger positions once; anything that changes page
// height afterwards (web fonts settling, FAQ <details> opening) leaves the
// spine's end position and the pin's range stale. This invisible component
// re-measures at those moments.
export function ScrollRefresh() {
  useEffect(() => {
    let cancelled = false;
    document.fonts?.ready.then(() => {
      if (!cancelled) ScrollTrigger.refresh();
    });

    const onToggle = (e: Event) => {
      if ((e.target as HTMLElement).tagName === "DETAILS") {
        ScrollTrigger.refresh();
      }
    };
    // "toggle" doesn't bubble, but it IS retargetable via capture.
    document.addEventListener("toggle", onToggle, true);
    return () => {
      cancelled = true;
      document.removeEventListener("toggle", onToggle, true);
    };
  }, []);
  return null;
}
