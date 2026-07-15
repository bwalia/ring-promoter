"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { RingMark } from "@/components/ring-mark";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api, ApiError } from "@/lib/api";
import { useAuthStore } from "@/lib/stores";

/** Full-screen gate shown until a valid API token is stored. */
export function TokenGate() {
  const setToken = useAuthStore((s) => s.setToken);
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const token = value.trim();
    if (!token) return;
    setChecking(true);
    setError(null);
    try {
      const ok = await api.validateToken(token);
      if (!ok) {
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
    <div className="flex min-h-dvh items-center justify-center bg-muted/40 p-4">
      <form
        onSubmit={submit}
        className="w-full max-w-sm space-y-4 rounded-xl border bg-card p-8 shadow-sm"
      >
        <div className="flex flex-col items-center gap-2 text-center">
          <div className="flex size-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
            <RingMark className="size-7" />
          </div>
          <h1 className="font-display text-xl font-bold tracking-tight">Ring Promoter</h1>
          <p className="text-sm text-muted-foreground">
            Enter your API token to connect.
          </p>
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
    </div>
  );
}
