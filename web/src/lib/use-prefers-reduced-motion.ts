"use client";

import { useSyncExternalStore } from "react";

const QUERY = "(prefers-reduced-motion: reduce)";

function subscribe(onChange: () => void): () => void {
  const mq = window.matchMedia(QUERY);
  mq.addEventListener("change", onChange);
  return () => mq.removeEventListener("change", onChange);
}

function getSnapshot(): boolean {
  return window.matchMedia(QUERY).matches;
}

/** On the server nothing is animating yet, so report "motion allowed". */
function getServerSnapshot(): boolean {
  return false;
}

/**
 * Tracks the user's motion preference, live.
 *
 * useSyncExternalStore rather than useState+useEffect: the media query is an
 * external store, so this reads the real value on the first client render
 * (no post-hydration flash of animation) and stays subscribed to changes.
 */
export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
