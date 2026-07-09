"use client";

import { useSyncExternalStore } from "react";

/**
 * The current time, quantized to `stepMs` buckets so the snapshot is stable
 * between ticks (a raw Date.now() in render is impure and re-renders forever).
 * Returns 0 during server prerender.
 */
export function useNow(stepMs = 5_000): number {
  return useSyncExternalStore(
    (onTick) => {
      const t = setInterval(onTick, stepMs);
      return () => clearInterval(t);
    },
    () => Math.floor(Date.now() / stepMs) * stepMs,
    () => 0,
  );
}
