"use client";

import { useState } from "react";
import { Loader2, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAppTitle, useStartPipeline } from "@/lib/queries";
import type { PipelineDef, PipelineRun } from "@/lib/types";
import { cn } from "@/lib/utils";

const RINGS = ["int", "test", "acc", "prod"] as const;

/** Apps deployed by a pipeline, in stage order, de-duplicated. */
export function pipelineApps(p: PipelineDef): string[] {
  const seen = new Set<string>();
  for (const s of p.stages) {
    if (s.type !== "deploy") continue;
    for (const a of s.apps ?? []) seen.add(a);
  }
  return [...seen];
}

/**
 * Start a pipeline run. Into int every app is SEEDED (a version per app, or
 * the ring's pinned ref when left blank); into later rings each app is
 * PROMOTED from the ring before.
 */
export function RunPipelineDialog({
  pipeline,
  open,
  onOpenChange,
  onStarted,
}: {
  pipeline: PipelineDef;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStarted: (run: PipelineRun) => void;
}) {
  const title = useAppTitle();
  const [ring, setRing] = useState<string>("int");
  const [versions, setVersions] = useState<Record<string, string>>({});
  const [cr, setCr] = useState("");
  const [password, setPassword] = useState("");
  const start = useStartPipeline(pipeline.name, (run) => {
    onOpenChange(false);
    onStarted(run);
  });
  const apps = pipelineApps(pipeline);

  const submit = () => {
    const v = Object.fromEntries(
      Object.entries(versions)
        .map(([k, val]) => [k, val.trim()])
        .filter(([, val]) => val),
    );
    start.mutate({
      ring,
      ...(ring === "int" && Object.keys(v).length ? { versions: v } : {}),
      ...(cr.trim() ? { change_request_code: cr.trim() } : {}),
      ...(ring === "prod" && password ? { password } : {}),
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Run {pipeline.display_name ?? pipeline.name}</DialogTitle>
          <DialogDescription>
            Stages run in dependency order; apps inside a stage deploy in
            parallel. Gates and approvals hold the run until they say Go.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label>Target ring</Label>
            <div className="grid grid-cols-4 gap-1.5" role="radiogroup" aria-label="Target ring">
              {RINGS.map((r) => (
                <button
                  key={r}
                  type="button"
                  role="radio"
                  aria-checked={ring === r}
                  onClick={() => setRing(r)}
                  className={cn(
                    "rounded-md border px-2 py-1.5 font-mono text-sm transition-colors",
                    ring === r
                      ? "border-primary bg-primary/10 text-foreground"
                      : "text-muted-foreground hover:bg-muted",
                  )}
                >
                  {r}
                </button>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              {ring === "int"
                ? "Seeds every app into int."
                : `Promotes every app from the ring before into ${ring}.`}
            </p>
          </div>

          {ring === "int" && apps.length > 0 && (
            <div className="space-y-1.5">
              <Label>Versions</Label>
              <p className="text-xs text-muted-foreground">
                Leave blank to use the version int is pinned to in config.
              </p>
              <div className="max-h-56 space-y-1.5 overflow-y-auto pr-1">
                {apps.map((a) => (
                  <div key={a} className="flex items-center gap-2">
                    <span className="w-40 shrink-0 truncate text-sm" title={a}>
                      {title(a)}
                    </span>
                    <Input
                      className="h-8 font-mono text-xs"
                      placeholder="tag, branch or sha"
                      value={versions[a] ?? ""}
                      onChange={(e) =>
                        setVersions((s) => ({ ...s, [a]: e.target.value }))
                      }
                      aria-label={`Version for ${title(a)}`}
                    />
                  </div>
                ))}
              </div>
            </div>
          )}

          {(ring === "acc" || ring === "prod") && (
            <div className="space-y-1.5">
              <Label htmlFor="pipeline-cr">Change request (if the policy needs one)</Label>
              <Input
                id="pipeline-cr"
                className="h-8 font-mono text-xs"
                value={cr}
                onChange={(e) => setCr(e.target.value)}
              />
            </div>
          )}

          {ring === "prod" && (
            <div className="space-y-1.5">
              <Label htmlFor="pipeline-password">Production password</Label>
              <Input
                id="pipeline-password"
                type="password"
                autoComplete="off"
                className="h-8"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={submit}
            disabled={start.isPending || (ring === "prod" && !password)}
          >
            {start.isPending ? (
              <Loader2 aria-hidden className="size-4 animate-spin" />
            ) : (
              <Play aria-hidden className="size-4" />
            )}
            Run into {ring}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
