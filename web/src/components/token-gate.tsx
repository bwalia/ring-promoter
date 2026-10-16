"use client";

import { useState } from "react";
import { CircleDot, Loader2, Plus } from "lucide-react";
import { ClusterDot, clusterHost, useSwitchCluster } from "@/components/clusters";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api, ApiError } from "@/lib/api";
import { activeCluster, useAuthStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";

/**
 * Full-screen gate shown until the active cluster has a valid API token. With
 * several clusters saved, it also offers the way to another one, so a token
 * that expired on one server never strands the operator.
 */
export function TokenGate() {
  const setToken = useAuthStore((s) => s.setToken);
  const cluster = useAuthStore(activeCluster);
  const clusters = useAuthStore((s) => s.clusters);
  const tokens = useAuthStore((s) => s.tokens);
  const setClusterDialog = useUiStore((s) => s.setClusterDialog);
  const switchTo = useSwitchCluster();
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);

  const others = clusters.filter((c) => c.id !== cluster.id);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const token = value.trim();
    if (!token) return;
    setChecking(true);
    setError(null);
    try {
      const apps = await api.validateToken(token);
      if (apps === null) {
        setError("That token was rejected. Check it and try again.");
        return;
      }
      setToken(token);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Could not reach the server. Is it running?",
      );
    } finally {
      setChecking(false);
    }
  };

  return (
    <div className="rp-console flex min-h-dvh items-center justify-center bg-muted/40 p-4">
      <div className="w-full max-w-sm space-y-4">
        <form
          onSubmit={submit}
          className="space-y-4 rounded-xl border bg-card p-8 shadow-sm"
        >
          <div className="flex flex-col items-center gap-2 text-center">
            <div className="flex size-12 items-center justify-center rounded-full bg-primary text-primary-foreground">
              <CircleDot aria-hidden className="size-6" />
            </div>
            <h1 className="rp-heading-section text-balance">Ring Promoter</h1>
            <p className="text-[length:var(--rp-font-lead)] text-muted-foreground">
              Enter your API token to connect.
            </p>
            {clusters.length > 1 && (
              <p className="flex items-center gap-1.5 text-sm">
                <ClusterDot color={cluster.color} />
                <span className="font-medium">{cluster.name}</span>
                <span className="font-mono text-xs text-muted-foreground">
                  {clusterHost(cluster)}
                </span>
              </p>
            )}
          </div>
          <Input
            type="password"
            autoFocus
            autoComplete="off"
            placeholder="API token"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            aria-label="API token"
          />
          {error && <p className="text-sm text-status-critical">{error}</p>}
          <Button type="submit" className="w-full" disabled={checking || !value.trim()}>
            {checking && <Loader2 aria-hidden className="size-4 animate-spin" />}
            Connect
          </Button>
        </form>

        <div className="rounded-xl border bg-card p-4 shadow-sm">
          {others.length > 0 && (
            <>
              <p className="mb-2 text-xs font-medium text-muted-foreground">
                Or switch cluster
              </p>
              <ul className="mb-2 space-y-1">
                {others.map((c) => (
                  <li key={c.id}>
                    <button
                      type="button"
                      onClick={() => switchTo(c.id)}
                      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
                    >
                      <ClusterDot color={c.color} />
                      <span className="truncate">{c.name}</span>
                      <span className="ml-auto truncate font-mono text-xs text-muted-foreground">
                        {tokens[c.id] ? clusterHost(c) : "no token"}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="w-full"
            onClick={() => setClusterDialog({ mode: "add" })}
          >
            <Plus aria-hidden className="size-4" /> Add another cluster
          </Button>
        </div>
      </div>
    </div>
  );
}
