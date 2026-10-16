"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  ChevronsUpDown,
  KeyRound,
  Loader2,
  Pencil,
  Plus,
  Server,
  Trash2,
  WifiOff,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api, ApiError } from "@/lib/api";
import {
  activeCluster,
  CLUSTER_COLORS,
  defaultClusterName,
  LOCAL_CLUSTER_ID,
  normaliseClusterUrl,
  useAuthStore,
  usePrefsStore,
  type Cluster,
  type ClusterColor,
} from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";
import { cn } from "@/lib/utils";

// Literal class names so Tailwind generates them; the colours are the theme's
// own chart tokens.
const DOT: Record<ClusterColor, string> = {
  "chart-1": "bg-chart-1",
  "chart-2": "bg-chart-2",
  "chart-3": "bg-chart-3",
  "chart-4": "bg-chart-4",
  "chart-5": "bg-chart-5",
};

export function ClusterDot({
  color,
  className,
}: {
  color: ClusterColor;
  className?: string;
}) {
  return (
    <span
      aria-hidden
      className={cn("inline-block size-2.5 shrink-0 rounded-full", DOT[color], className)}
    />
  );
}

/** Host shown under a cluster's name. */
export function clusterHost(c: Cluster): string {
  if (!c.url) return typeof window === "undefined" ? "" : window.location.host;
  return c.url.replace(/^https?:\/\//, "");
}

/**
 * Switch the console to another cluster: drop every cached response from the
 * old one (so nothing from it can show under the new name) and return to the
 * overview, since the open app or group may not exist there.
 */
export function useSwitchCluster() {
  const queryClient = useQueryClient();
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  return async (id: string) => {
    const auth = useAuthStore.getState();
    if (id === auth.activeId) return;
    const target = auth.clusters.find((c) => c.id === id);
    if (!target) return;
    await queryClient.cancelQueries();
    auth.switchCluster(id);
    queryClient.clear();
    selectGroup(null);
    toast.success(`Switched to ${target.name}`, { description: clusterHost(target) });
  };
}

/** The top-bar menu: which cluster am I on, and jump to another. */
export function ClusterSwitcher({ className }: { className?: string }) {
  const clusters = useAuthStore((s) => s.clusters);
  const activeId = useAuthStore((s) => s.activeId);
  const active = useAuthStore(activeCluster);
  const setClusterDialog = useUiStore((s) => s.setClusterDialog);
  const setClustersOpen = useUiStore((s) => s.setClustersOpen);
  const switchTo = useSwitchCluster();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn("h-8 max-w-[14rem] gap-2 px-2.5", className)}
          aria-label={`Cluster: ${active.name}. Switch cluster`}
        >
          <ClusterDot color={active.color} />
          <span className="truncate">{active.name}</span>
          <ChevronsUpDown aria-hidden className="size-3.5 shrink-0 opacity-50" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-64">
        <DropdownMenuLabel>Clusters</DropdownMenuLabel>
        {clusters.map((c) => (
          <DropdownMenuItem key={c.id} onClick={() => switchTo(c.id)}>
            <ClusterDot color={c.color} />
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate">{c.name}</span>
              <span className="truncate font-mono text-[11px] text-muted-foreground">
                {clusterHost(c)}
              </span>
            </span>
            {c.id === activeId && <Check aria-hidden className="size-4" />}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => setClusterDialog({ mode: "add" })}>
          <Plus aria-hidden className="size-4" /> Add a cluster…
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setClustersOpen(true)}>
          <Server aria-hidden className="size-4" /> Manage clusters…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Mounted once; renders whichever cluster dialog is open. */
export function ClusterDialogs() {
  const dialog = useUiStore((s) => s.clusterDialog);
  const setClusterDialog = useUiStore((s) => s.setClusterDialog);
  const clustersOpen = useUiStore((s) => s.clustersOpen);
  const setClustersOpen = useUiStore((s) => s.setClustersOpen);
  const clusters = useAuthStore((s) => s.clusters);
  const editing =
    dialog?.mode === "edit" ? clusters.find((c) => c.id === dialog.id) : undefined;

  return (
    <>
      <ManageClustersDialog open={clustersOpen} onOpenChange={setClustersOpen} />
      {dialog && (
        <ClusterFormDialog
          key={dialog.mode === "edit" ? dialog.id : "add"}
          editing={editing}
          onClose={() => setClusterDialog(null)}
        />
      )}
    </>
  );
}

function nextUnusedColor(clusters: Cluster[]): ClusterColor {
  const used = new Set(clusters.map((c) => c.color));
  return CLUSTER_COLORS.find((c) => !used.has(c)) ?? CLUSTER_COLORS[clusters.length % CLUSTER_COLORS.length];
}

/**
 * Add or edit a cluster. The token is checked against the server before it
 * is saved; when editing, a blank token keeps the saved one so renaming never
 * means pasting a credential again.
 */
function ClusterFormDialog({
  editing,
  onClose,
}: {
  editing?: Cluster;
  onClose: () => void;
}) {
  const clusters = useAuthStore((s) => s.clusters);
  const tokens = useAuthStore((s) => s.tokens);
  const saveCluster = useAuthStore((s) => s.saveCluster);
  const switchTo = useSwitchCluster();
  const queryClient = useQueryClient();
  const isLocal = editing?.id === LOCAL_CLUSTER_ID;

  const [url, setUrl] = useState(editing?.url ?? "");
  const [name, setName] = useState(editing?.name ?? "");
  const [token, setToken] = useState("");
  const [color, setColor] = useState<ClusterColor>(
    editing?.color ?? nextUnusedColor(clusters),
  );
  const [switchAfter, setSwitchAfter] = useState(!editing);
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);

  const origin = isLocal ? "" : normaliseClusterUrl(url);
  const savedToken = editing ? tokens[editing.id] : undefined;
  const placeholderName = origin ? defaultClusterName(origin) : "e.g. Production";
  const insecure =
    !!origin &&
    origin.startsWith("http:") &&
    typeof window !== "undefined" &&
    window.location.protocol === "https:";
  const duplicate =
    !!origin && clusters.some((c) => c.url === origin && c.id !== editing?.id);
  const canSubmit =
    (isLocal || !!origin) &&
    !duplicate &&
    (!!token.trim() || !!savedToken) &&
    !checking;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit || origin === null) return;
    const credential = token.trim() || savedToken!;
    setChecking(true);
    setError(null);
    try {
      const apps = await api.validateToken(credential, origin);
      if (apps === null) {
        setError("That token was rejected by the server.");
        return;
      }
      const cluster: Cluster = {
        id: editing?.id ?? crypto.randomUUID(),
        name: name.trim() || (isLocal ? "This server" : defaultClusterName(origin)),
        url: origin,
        color,
      };
      const repointed =
        !!editing &&
        editing.id === useAuthStore.getState().activeId &&
        (editing.url !== origin || !!token.trim());
      if (repointed) await queryClient.cancelQueries();
      saveCluster(cluster, token.trim() || null);
      // The active cluster now answers from a new address or token.
      if (repointed) queryClient.clear();
      if (!editing) {
        toast.success(`Added ${cluster.name}`, {
          description: `${apps} application${apps === 1 ? "" : "s"} available.`,
        });
      }
      onClose();
      if (switchAfter) await switchTo(cluster.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setChecking(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{editing ? `Edit ${editing.name}` : "Add a cluster"}</DialogTitle>
            <DialogDescription>
              Another Ring Promoter server, reached with its own API token. Switch
              between clusters any time from the top bar.
            </DialogDescription>
          </DialogHeader>

          {!isLocal && (
            <div className="space-y-2">
              <Label htmlFor="cluster-url">Server URL</Label>
              <Input
                id="cluster-url"
                autoFocus={!editing}
                autoComplete="off"
                spellCheck={false}
                placeholder="https://ring-promoter.example.com"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                className="font-mono"
              />
              {duplicate && (
                <p className="text-xs text-status-warning">
                  That server is already saved.
                </p>
              )}
              {insecure && (
                <p className="text-xs text-status-warning">
                  This console is on HTTPS, so the browser will block a plain-HTTP
                  server.
                </p>
              )}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="cluster-name">Name</Label>
            <Input
              id="cluster-name"
              autoComplete="off"
              placeholder={placeholderName}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="cluster-token">API token</Label>
            <Input
              id="cluster-token"
              type="password"
              autoComplete="off"
              placeholder={savedToken ? "Unchanged" : "Bearer token"}
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {savedToken ? "Leave blank to keep the saved token. " : ""}
              Kept in this browser only.
            </p>
          </div>

          <div className="space-y-2">
            <Label>Colour</Label>
            <div className="flex gap-2" role="radiogroup" aria-label="Colour">
              {CLUSTER_COLORS.map((c) => (
                <button
                  key={c}
                  type="button"
                  role="radio"
                  aria-checked={color === c}
                  aria-label={`Colour ${c.replace("chart-", "")}`}
                  onClick={() => setColor(c)}
                  className={cn(
                    "flex size-7 items-center justify-center rounded-full border",
                    color === c ? "ring-2 ring-ring ring-offset-2 ring-offset-background" : "",
                  )}
                >
                  <ClusterDot color={c} className="size-4" />
                </button>
              ))}
            </div>
          </div>

          {editing && editing.id !== useAuthStore.getState().activeId && (
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={switchAfter}
                onChange={(e) => setSwitchAfter(e.target.checked)}
              />
              Switch to it after saving
            </label>
          )}
          {!editing && (
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={switchAfter}
                onChange={(e) => setSwitchAfter(e.target.checked)}
              />
              Switch to it now
            </label>
          )}

          {error && <p className="text-sm text-status-critical">{error}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              {checking && <Loader2 aria-hidden className="size-4 animate-spin" />}
              {editing ? "Save" : "Connect"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

type Probe =
  | { state: "ok"; apps: number }
  | { state: "no-token" }
  | { state: "rejected" }
  | { state: "unreachable"; message: string };

/** One cheap call per cluster with its own token: GET /api/apps. */
function useProbe(cluster: Cluster, token: string | undefined, enabled: boolean) {
  return useQuery<Probe>({
    // Not keyed under the active cluster's queries; cleared with them on switch.
    queryKey: ["cluster-probe", cluster.id, cluster.url, token ?? ""],
    enabled,
    staleTime: 15_000,
    retry: false,
    queryFn: async () => {
      if (!token) return { state: "no-token" };
      try {
        const apps = await api.validateToken(token, cluster.url);
        return apps === null ? { state: "rejected" } : { state: "ok", apps };
      } catch (err) {
        return {
          state: "unreachable",
          message: err instanceof ApiError ? err.message : String(err),
        };
      }
    },
  });
}

function ManageClustersDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const clusters = useAuthStore((s) => s.clusters);
  const setClusterDialog = useUiStore((s) => s.setClusterDialog);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Clusters</DialogTitle>
          <DialogDescription>
            Every Ring Promoter server saved in this browser, and how each one is
            doing right now.
          </DialogDescription>
        </DialogHeader>
        <ul className="-mx-2 max-h-[60vh] divide-y overflow-y-auto">
          {clusters.map((c) => (
            <ClusterRow key={c.id} cluster={c} open={open} onSwitched={() => onOpenChange(false)} />
          ))}
        </ul>
        <DialogFooter>
          <Button variant="outline" onClick={() => setClusterDialog({ mode: "add" })}>
            <Plus aria-hidden className="size-4" /> Add a cluster
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ClusterRow({
  cluster,
  open,
  onSwitched,
}: {
  cluster: Cluster;
  open: boolean;
  onSwitched: () => void;
}) {
  const activeId = useAuthStore((s) => s.activeId);
  const token = useAuthStore((s) => s.tokens[cluster.id]);
  const removeCluster = useAuthStore((s) => s.removeCluster);
  const setClusterDialog = useUiStore((s) => s.setClusterDialog);
  const switchTo = useSwitchCluster();
  const probe = useProbe(cluster, token, open);
  const isActive = cluster.id === activeId;
  const [confirming, setConfirming] = useState(false);

  return (
    <li className="flex items-start gap-3 px-2 py-3">
      <ClusterDot color={cluster.color} className="mt-1.5" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate font-medium">{cluster.name}</span>
          {isActive && (
            <span className="rounded bg-muted px-1.5 text-[11px] text-muted-foreground">
              current
            </span>
          )}
        </div>
        <div className="truncate font-mono text-xs text-muted-foreground">
          {clusterHost(cluster)}
        </div>
        <div className="mt-1 text-xs">
          {probe.isPending ? (
            <span className="inline-flex items-center gap-1 text-muted-foreground">
              <Loader2 aria-hidden className="size-3 animate-spin" /> Checking…
            </span>
          ) : probe.data?.state === "ok" ? (
            <span className="text-status-good">
              Reachable · {probe.data.apps} app{probe.data.apps === 1 ? "" : "s"}
            </span>
          ) : probe.data?.state === "no-token" ? (
            <span className="inline-flex items-center gap-1 text-status-warning">
              <KeyRound aria-hidden className="size-3" /> No token saved
            </span>
          ) : probe.data?.state === "rejected" ? (
            <span className="inline-flex items-center gap-1 text-status-critical">
              <KeyRound aria-hidden className="size-3" /> Token rejected
            </span>
          ) : probe.data?.state === "unreachable" ? (
            <span className="inline-flex items-start gap-1 text-status-critical">
              <WifiOff aria-hidden className="mt-0.5 size-3 shrink-0" />
              {probe.data.message}
            </span>
          ) : null}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {!isActive && (
          <Button
            size="sm"
            variant="secondary"
            onClick={async () => {
              onSwitched();
              await switchTo(cluster.id);
            }}
          >
            Switch
          </Button>
        )}
        <Button
          size="icon"
          variant="ghost"
          className="size-8"
          aria-label={`Edit ${cluster.name}`}
          onClick={() => setClusterDialog({ mode: "edit", id: cluster.id })}
        >
          <Pencil aria-hidden className="size-4" />
        </Button>
        {cluster.id !== LOCAL_CLUSTER_ID &&
          (confirming ? (
            <Button
              size="sm"
              variant="destructive"
              onClick={async () => {
                // Leave it first, so its cached data goes with it.
                if (isActive) await switchTo(LOCAL_CLUSTER_ID);
                removeCluster(cluster.id);
                toast(`Removed ${cluster.name}`);
              }}
            >
              Remove
            </Button>
          ) : (
            <Button
              size="icon"
              variant="ghost"
              className="size-8"
              aria-label={`Remove ${cluster.name}`}
              onClick={() => setConfirming(true)}
            >
              <Trash2 aria-hidden className="size-4" />
            </Button>
          ))}
      </div>
    </li>
  );
}
