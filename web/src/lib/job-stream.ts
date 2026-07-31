"use client";

import { useEffect, useSyncExternalStore } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@/lib/stores";
import type { Job } from "@/lib/types";

// Server-push replacement for the 2-second /api/jobs poll: one long-lived SSE
// connection receives a full jobs snapshot whenever anything changes and
// nothing while the system is idle. While connected, useJobs() switches its
// polling off; if the stream drops (proxy, redeploy, network) polling resumes
// until the automatic reconnect lands.
//
// Uses fetch + ReadableStream instead of EventSource because the API expects
// a bearer token in the Authorization header, which EventSource cannot set.

let connected = false;
const listeners = new Set<() => void>();

function setConnected(value: boolean) {
  if (value === connected) return;
  connected = value;
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Whether the live job stream is currently connected. */
export function useJobStreamConnected(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => connected,
    () => false,
  );
}

const RECONNECT_MIN_MS = 1_000;
const RECONNECT_MAX_MS = 30_000;

/** One SSE frame: `event:`/`data:` lines up to a blank line. */
function parseFrame(raw: string): { event: string; data: string } {
  let event = "message";
  const data: string[] = [];
  for (const line of raw.split("\n")) {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
  }
  return { event, data: data.join("\n") };
}

/**
 * Maintains the /api/events connection for the lifetime of the signed-in app.
 * Mount exactly once (Providers does). Snapshots land in the ["jobs"] query
 * cache, so every existing consumer — job cards, toasts, ring/history
 * invalidation — keeps working unchanged.
 */
export function useJobStream() {
  const token = useAuthStore((s) => s.token);
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!token) return;
    let stopped = false;
    let controller: AbortController | null = null;

    const handleFrame = (raw: string) => {
      const { event, data } = parseFrame(raw);
      if (event !== "jobs" || !data) return;
      try {
        const payload = JSON.parse(data) as { jobs: Job[] };
        queryClient.setQueryData(["jobs"], payload);
      } catch {
        // A malformed frame is superseded by the next snapshot.
      }
    };

    const run = async () => {
      let retry = RECONNECT_MIN_MS;
      while (!stopped) {
        controller = new AbortController();
        try {
          const res = await fetch("/api/events", {
            headers: {
              Authorization: `Bearer ${token}`,
              Accept: "text/event-stream",
            },
            cache: "no-store",
            signal: controller.signal,
          });
          if (res.status === 401) {
            // Token rejected: the api client's next call signs the user out;
            // don't hammer the endpoint from here.
            return;
          }
          if (!res.ok || !res.body) {
            throw new Error(`stream failed (${res.status})`);
          }
          setConnected(true);
          retry = RECONNECT_MIN_MS;
          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buffer = "";
          for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            let end;
            while ((end = buffer.indexOf("\n\n")) >= 0) {
              handleFrame(buffer.slice(0, end));
              buffer = buffer.slice(end + 2);
            }
          }
        } catch {
          // Connection error — fall through to the backoff below.
        }
        setConnected(false);
        if (stopped) return;
        await new Promise((r) => setTimeout(r, retry));
        retry = Math.min(retry * 2, RECONNECT_MAX_MS);
      }
    };
    run();

    return () => {
      stopped = true;
      controller?.abort();
      setConnected(false);
    };
  }, [token, queryClient]);
}
