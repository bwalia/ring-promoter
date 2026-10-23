"use client";

import { useMemo, useState } from "react";
import {
  Ban,
  Check,
  FileDown,
  Loader2,
  Play,
  RotateCcw,
  Search,
  Workflow,
  X,
} from "lucide-react";
import {
  FlowGraph,
  STATUS_LABEL,
  StatusIcon,
  VerdictPill,
  gateVerdict,
} from "@/components/flow/flow-graph";
import { JobFlow } from "@/components/flow/job-flow";
import { RunPipelineDialog } from "@/components/pipelines/run-pipeline-dialog";
import { RelativeTime } from "@/components/relative-time";
import { SecurityChecksList, countsMeta } from "@/components/security/security-checks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import type { FlowNode, FlowStatus } from "@/lib/flow-layout";
import {
  useAppTitle,
  useCancelPipelineRun,
  useDecidePipelineStage,
  useJob,
  usePipelineRun,
  usePipelineRuns,
  useRerunPipeline,
} from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import { duration } from "@/lib/time";
import type {
  PipelineDef,
  PipelineRun,
  PipelineRunItem,
  PipelineRunStage,
  PipelineStageDef,
} from "@/lib/types";
import { SEVERITIES } from "@/lib/types";
import { cn } from "@/lib/utils";

const terminal = (s: string) => s === "success" || s === "failed" || s === "cancelled";

const stageLabel = (s: { name: string; display_name?: string }) =>
  s.display_name || s.name;

/** A not-yet-run stage, so the definition reads as a blueprint graph. */
function blueprintStage(s: PipelineStageDef): PipelineRunStage {
  return {
    name: s.name,
    display_name: s.display_name,
    type: s.type,
    gate: s.gate,
    needs: s.needs,
    status: "queued",
    items: (s.apps ?? []).map((app) => ({ app, status: "queued" as const })),
  };
}

/** "2C 1H" or "sca missing" — why a security gate item is where it is. */
function securityMeta(i: PipelineRunItem): string | undefined {
  const checks = i.checks ?? [];
  if (!checks.length) return undefined;
  const missing = checks.filter((c) => c.status === "missing").map((c) => c.kind);
  if (missing.length) return `${missing.join(",")} missing`;
  const total = { critical: 0, high: 0, medium: 0, low: 0 };
  for (const c of checks) for (const k of SEVERITIES) total[k] += c[k] ?? 0;
  return countsMeta(total) || "clean";
}

/**
 * Save a run's compliance evidence: the controls it passed through (QA and
 * AppSec gates with their per-service verdicts and scan checks, approvals and
 * who gave them) as a JSON file an auditor can keep.
 */
function downloadEvidence(run: PipelineRun) {
  const evidence = {
    kind: "ring-promoter/pipeline-evidence@v1",
    generated_at: new Date().toISOString(),
    run: {
      id: run.id,
      pipeline: run.pipeline,
      ring: run.ring,
      status: run.status,
      triggered_by: run.triggered_by,
      rerun_of: run.rerun_of,
      started_at: run.started_at,
      finished_at: run.finished_at,
    },
    controls: run.stages
      .filter((s) => s.type !== "deploy")
      .map((s) => ({
        stage: s.name,
        type: s.type,
        gate: s.gate,
        status: s.status,
        decision: s.message,
        approved_by: s.approved_by,
        decided_at: s.finished_at,
        items: (s.items ?? []).map((i) => ({
          app: i.app,
          version: i.version,
          verdict: i.verdict,
          detail: i.detail,
          checks: i.checks,
        })),
      })),
    deployments: run.stages
      .filter((s) => s.type === "deploy")
      .flatMap((s) =>
        (s.items ?? []).map((i) => ({
          stage: s.name,
          app: i.app,
          version: i.version,
          status: i.status,
          job_id: i.job_id,
        })),
      ),
  };
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(evidence, null, 2)], { type: "application/json" }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = `${run.pipeline}-${run.id}-evidence.json`;
  a.click();
  URL.revokeObjectURL(url);
}

function runToNodes(
  pipeline: PipelineDef,
  run: PipelineRun | undefined,
  title: (a: string) => string,
): FlowNode[] {
  const stages = run?.stages ?? pipeline.stages.map(blueprintStage);
  const hasDownstream = new Set(stages.flatMap((s) => s.needs ?? []));
  const nodes: FlowNode[] = [
    {
      id: "trigger",
      kind: "trigger",
      label: run ? `Run ${run.id}` : "Trigger",
      sublabel: run
        ? [run.triggered_by, `→ ${run.ring}`].filter(Boolean).join(" ")
        : "manual or API",
      status: run ? "success" : "queued",
      startedAt: run?.started_at,
      needs: [],
    },
  ];

  for (const s of stages) {
    const needs = s.needs?.length ? s.needs.map((n) => `stage:${n}`) : ["trigger"];
    const items = s.items ?? [];
    const base = {
      id: `stage:${s.name}`,
      label: stageLabel(s),
      status: s.status as FlowStatus,
      startedAt: s.started_at,
      finishedAt: s.finished_at,
      needs,
      keywords: `${s.name} ${items.map((i) => `${i.app} ${title(i.app)}`).join(" ")}`,
    };
    if (s.type === "gate") {
      const verdicts = items.map((i) => i.verdict ?? "unknown");
      const security = s.gate === "security";
      nodes.push({
        ...base,
        kind: "gate",
        gate: security ? "security" : "qa",
        sublabel: s.message ?? (security ? "AppSec scans Go / No-Go" : "QA Go / No-Go"),
        verdict: verdicts.includes("no_go")
          ? "no_go"
          : s.status === "success" || s.status === "reused"
            ? "go"
            : undefined,
        children: items.map((i) => ({
          id: i.app,
          label: title(i.app),
          sublabel: i.detail,
          meta: security ? securityMeta(i) : undefined,
          status: i.status,
          verdict: i.verdict ?? "unknown",
        })),
      });
    } else if (s.type === "approval") {
      nodes.push({
        ...base,
        kind: "approval",
        sublabel: s.approved_by
          ? `${s.status === "failed" ? "rejected" : "approved"} by ${s.approved_by}`
          : (s.message ?? "manual approval"),
      });
    } else {
      nodes.push({
        ...base,
        kind: "task",
        sublabel: `${items.length} service${items.length === 1 ? "" : "s"} · parallel`,
        children: items.map((i) => ({
          id: i.app,
          label: title(i.app),
          sublabel: i.version,
          status: i.status,
          startedAt: undefined,
        })),
      });
    }
  }

  const leaves = stages.filter((s) => !hasDownstream.has(s.name)).map((s) => `stage:${s.name}`);
  nodes.push({
    id: "done",
    kind: "terminal",
    label: run ? (run.status === "success" ? `Released to ${run.ring}` : STATUS_LABEL[run.status]) : "Release",
    status: !run
      ? "queued"
      : run.status === "success"
        ? "success"
        : run.status === "failed" || run.status === "cancelled"
          ? "failed"
          : "queued",
    finishedAt: run?.finished_at,
    needs: leaves.length ? leaves : ["trigger"],
  });
  return nodes;
}

export function PipelineView({ pipeline }: { pipeline: PipelineDef }) {
  const title = useAppTitle();
  const runs = usePipelineRuns(pipeline.name);
  const [runId, setRunId] = useState<string | null>(null);
  const activeId = runId ?? runs.data?.[0]?.id ?? null;
  const runQuery = usePipelineRun(activeId);
  const run =
    runQuery.data ?? runs.data?.find((r) => r.id === activeId) ?? undefined;
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [dialog, setDialog] = useState(false);
  const decide = useDecidePipelineStage();
  const cancel = useCancelPipelineRun();
  const rerun = useRerunPipeline((r) => setRunId(r.id));

  const nodes = useMemo(() => runToNodes(pipeline, run, title), [pipeline, run, title]);
  const serviceCount = new Set(
    pipeline.stages.flatMap((s) => s.apps ?? []),
  ).size;

  const renderActions = (n: FlowNode) => {
    if (n.kind !== "approval" || !run || n.status !== "waiting") return null;
    const stage = n.id.slice("stage:".length);
    const busy = decide.isPending;
    return (
      <div className="flex gap-1.5">
        <Button
          size="sm"
          className="h-7 flex-1"
          disabled={busy}
          onClick={() => decide.mutate({ id: run.id, stage, decision: "approve" })}
        >
          <Check aria-hidden className="size-3.5" /> Approve
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="h-7 flex-1"
          disabled={busy}
          onClick={() => decide.mutate({ id: run.id, stage, decision: "reject" })}
        >
          <X aria-hidden className="size-3.5" /> Reject
        </Button>
      </div>
    );
  };

  return (
    <div className="mx-auto max-w-7xl space-y-6 p-4 md:p-6 xl:p-8">
      <header className="flex flex-wrap items-start gap-4">
        <div className="flex size-10 items-center justify-center rounded-lg border bg-card">
          <Workflow aria-hidden className="size-5" />
        </div>
        <div className="min-w-0 flex-1">
          <h1 className="font-display text-xl font-bold tracking-tight">
            {pipeline.display_name ?? pipeline.name}
          </h1>
          <p className="text-sm text-muted-foreground">
            {pipeline.description ??
              `${pipeline.stages.length} stages · ${serviceCount} services`}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {run && terminal(run.status) && (
            <Button
              variant="outline"
              onClick={() => downloadEvidence(run)}
              title="Who ran it, every gate verdict, scan check and approval — for audits"
            >
              <FileDown aria-hidden className="size-4" /> Evidence
            </Button>
          )}
          {run && !terminal(run.status) && (
            <Button
              variant="outline"
              onClick={() => cancel.mutate(run.id)}
              disabled={cancel.isPending}
            >
              <Ban aria-hidden className="size-4" /> Cancel run
            </Button>
          )}
          {run && (run.status === "failed" || run.status === "cancelled") && (
            <Button
              variant="outline"
              onClick={() => {
                // A prod run needs the production password again.
                const password =
                  run.ring === "prod"
                    ? (window.prompt("Production password") ?? undefined)
                    : undefined;
                if (run.ring === "prod" && !password) return;
                rerun.mutate({ id: run.id, password });
              }}
              disabled={rerun.isPending}
            >
              <RotateCcw aria-hidden className="size-4" /> Re-run from failed
            </Button>
          )}
          <Button onClick={() => setDialog(true)}>
            <Play aria-hidden className="size-4" /> Run pipeline
          </Button>
        </div>
      </header>

      <section className="space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <h2 className="text-sm font-semibold">
            {run ? (
              <span className="flex items-center gap-2">
                <StatusIcon status={run.status} />
                Run {run.id} into <span className="font-mono">{run.ring}</span>
                <span className="font-normal text-muted-foreground">
                  · {STATUS_LABEL[run.status]} · {duration(run.started_at, run.finished_at)}
                  {run.rerun_of && ` · re-run of ${run.rerun_of}`}
                </span>
              </span>
            ) : (
              "Pipeline graph — not run yet"
            )}
          </h2>
          <div className="relative ml-auto w-full sm:w-64">
            <Search
              aria-hidden
              className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              placeholder="Search services or stages…"
              className="h-8 pl-8 text-sm"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              aria-label="Search services or stages"
            />
          </div>
        </div>
        <FlowGraph
          ariaLabel={`Graph of pipeline ${pipeline.name}`}
          nodes={nodes}
          query={query}
          selected={selected}
          onSelect={(id, child) => setSelected(child ? `${id}/${child}` : id)}
          renderActions={renderActions}
          className="pb-8"
        />
        {run?.error && (
          <p className="text-sm text-status-critical">{run.error}</p>
        )}
        {selected && (
          <SelectionDetail
            pipeline={pipeline}
            run={run}
            selected={selected}
            query={query}
            onSelect={setSelected}
          />
        )}
      </section>

      <RunHistory
        runs={runs.data}
        isPending={runs.isPending}
        activeId={activeId}
        onSelect={(id) => {
          setRunId(id);
          setSelected(null);
        }}
      />

      {dialog && (
        <RunPipelineDialog
          pipeline={pipeline}
          open
          onOpenChange={setDialog}
          onStarted={(r) => {
            setRunId(r.id);
            setSelected(null);
          }}
        />
      )}
    </div>
  );
}

function SelectionDetail({
  pipeline,
  run,
  selected,
  query,
  onSelect,
}: {
  pipeline: PipelineDef;
  run?: PipelineRun;
  selected: string;
  query: string;
  onSelect: (id: string) => void;
}) {
  const title = useAppTitle();
  const selectApp = usePrefsStore((s) => s.selectApp);
  const [, stageName, app] = /^stage:([^/]+)(?:\/(.+))?$/.exec(selected) ?? [];
  if (!stageName) return null;
  const stage =
    run?.stages.find((s) => s.name === stageName) ??
    (() => {
      const def = pipeline.stages.find((s) => s.name === stageName);
      return def ? blueprintStage(def) : undefined;
    })();
  if (!stage) return null;
  const item = app ? stage.items?.find((i) => i.app === app) : undefined;

  if (item && stage.type === "deploy") {
    return <ItemJob item={item} onOpenApp={() => selectApp(item.app)} />;
  }
  if (item && stage.type === "gate" && stage.gate === "security") {
    return (
      <div className="rounded-lg border bg-card">
        <div className="flex items-center gap-2 border-b px-3 py-2">
          <StatusIcon status={item.status} />
          <span className="flex-1 text-sm font-medium">
            {title(item.app)} · AppSec gate
          </span>
          {item.version && (
            <span className="font-mono text-xs text-muted-foreground">{item.version}</span>
          )}
          <VerdictPill verdict={item.verdict ?? "unknown"} />
          <Button variant="ghost" size="sm" className="h-7" onClick={() => selectApp(item.app)}>
            Open app
          </Button>
        </div>
        <div className="space-y-2 p-3">
          {item.detail && <p className="text-sm">{item.detail}</p>}
          <SecurityChecksList checks={item.checks} />
          <p className="text-xs text-muted-foreground">
            Fix the findings and re-scan, or record a time-boxed waiver on the app&apos;s Security
            panel. The gate re-reads scans until it times out.
          </p>
        </div>
      </div>
    );
  }

  // Stage overview: every member with a search — stages can hold dozens of
  // services, more than the graph box shows.
  const q = query.trim().toLowerCase();
  const items = (stage.items ?? []).filter(
    (i) => !q || i.app.toLowerCase().includes(q) || title(i.app).toLowerCase().includes(q),
  );
  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <StatusIcon status={stage.status} />
        <span className="flex-1 text-sm font-medium">{stageLabel(stage)}</span>
        {stage.type === "gate" && (
          <span className="rounded-sm bg-muted px-1 font-mono text-[9px] uppercase tracking-wider text-muted-foreground">
            {stage.gate === "security" ? "AppSec" : "QA"}
          </span>
        )}
        {stage.type === "gate" && <VerdictPill verdict={gateVerdict({ status: stage.status })} />}
        <span className="text-xs text-muted-foreground">
          {STATUS_LABEL[stage.status]}
          {stage.started_at && ` · ${duration(stage.started_at, stage.finished_at)}`}
        </span>
      </div>
      {stage.message && <p className="border-b px-3 py-2 text-sm">{stage.message}</p>}
      {stage.type === "approval" ? (
        <p className="px-3 py-2 text-sm text-muted-foreground">
          {stage.approved_by
            ? `Decided by ${stage.approved_by}.`
            : "Waits for a person to approve before dependent stages start."}
        </p>
      ) : (
        <ul className="max-h-80 divide-y overflow-y-auto">
          {items.map((i) => (
            <li key={i.app}>
              <button
                type="button"
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-muted/40"
                onClick={() => onSelect(`stage:${stage.name}/${i.app}`)}
              >
                <StatusIcon status={i.status} />
                <span className="min-w-0 flex-1 truncate">{title(i.app)}</span>
                {i.version && (
                  <span className="font-mono text-xs text-muted-foreground">{i.version}</span>
                )}
                {i.verdict && <VerdictPill verdict={i.verdict} />}
                {i.detail && (
                  <span className="max-w-[40%] truncate text-xs text-muted-foreground">
                    {i.detail}
                  </span>
                )}
              </button>
            </li>
          ))}
          {items.length === 0 && (
            <li className="px-3 py-2 text-sm text-muted-foreground">
              {q ? `No services match “${query}”.` : "No services in this stage."}
            </li>
          )}
        </ul>
      )}
    </div>
  );
}

/** One app's deploy inside a pipeline run, drawn with the per-job flow. */
function ItemJob({ item, onOpenApp }: { item: PipelineRunItem; onOpenApp: () => void }) {
  const title = useAppTitle();
  const job = useJob(item.app, item.job_id);
  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <div className="flex items-center gap-2 border-b bg-muted/40 px-3 py-2">
        <StatusIcon status={item.status} />
        <span className="flex-1 text-sm font-medium">{title(item.app)}</span>
        <Button variant="ghost" size="sm" className="h-7" onClick={onOpenApp}>
          Open app
        </Button>
      </div>
      {!item.job_id ? (
        <p className="px-3 py-3 text-sm text-muted-foreground">
          {item.status === "reused"
            ? "Reused from the earlier run — not deployed again."
            : item.status === "skipped"
              ? "Skipped — an upstream stage did not pass."
              : "Not started yet."}
        </p>
      ) : job.data ? (
        <JobFlow job={job.data} />
      ) : job.isPending ? (
        <div className="p-3">
          <Skeleton className="h-28 w-full" />
        </div>
      ) : (
        <p className="px-3 py-3 text-sm text-muted-foreground">
          This deploy is no longer in the server&apos;s memory (it keeps the
          newest 200). See the app&apos;s history instead.
        </p>
      )}
    </div>
  );
}

function RunHistory({
  runs,
  isPending,
  activeId,
  onSelect,
}: {
  runs?: PipelineRun[];
  isPending: boolean;
  activeId: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-semibold">Runs</h2>
      <div className="overflow-hidden rounded-lg border">
        {isPending ? (
          <div className="space-y-2 p-3">
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-full" />
          </div>
        ) : !runs?.length ? (
          <p className="p-3 text-sm text-muted-foreground">
            No runs yet — press Run pipeline.
          </p>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Run</th>
                <th className="px-3 py-2 font-medium">Ring</th>
                <th className="hidden px-3 py-2 font-medium sm:table-cell">Stages</th>
                <th className="px-3 py-2 font-medium">Duration</th>
                <th className="hidden px-3 py-2 font-medium md:table-cell">Started</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {runs.map((r) => {
                const done = r.stages.filter(
                  (s) => s.status === "success" || s.status === "reused",
                ).length;
                return (
                  <tr
                    key={r.id}
                    onClick={() => onSelect(r.id)}
                    className={cn(
                      "cursor-pointer hover:bg-muted/40",
                      r.id === activeId && "bg-muted/60",
                    )}
                  >
                    <td className="px-3 py-2">
                      <span className="flex items-center gap-1.5">
                        <StatusIcon status={r.status} />
                        <span className="hidden sm:inline">{STATUS_LABEL[r.status]}</span>
                      </span>
                    </td>
                    <td className="px-3 py-2 font-mono text-xs">
                      {r.id}
                      {r.rerun_of && (
                        <span className="ml-1 text-muted-foreground">↻ {r.rerun_of}</span>
                      )}
                    </td>
                    <td className="px-3 py-2 font-mono text-xs">{r.ring}</td>
                    <td className="hidden px-3 py-2 sm:table-cell">
                      <StageBar stages={r.stages} />
                      <span className="sr-only">
                        {done} of {r.stages.length} stages passed
                      </span>
                    </td>
                    <td className="px-3 py-2 font-mono text-xs">
                      {!terminal(r.status) && (
                        <Loader2 aria-hidden className="mr-1 inline size-3 animate-spin" />
                      )}
                      {duration(r.started_at, r.finished_at)}
                    </td>
                    <td className="hidden px-3 py-2 text-xs text-muted-foreground md:table-cell">
                      <RelativeTime iso={r.started_at} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

const BAR_TONE: Record<string, string> = {
  success: "bg-status-good",
  reused: "bg-status-good/50",
  failed: "bg-status-critical",
  running: "bg-primary animate-pulse",
  waiting: "bg-status-warning",
  queued: "bg-muted-foreground/30",
  skipped: "bg-muted-foreground/15",
  cancelled: "bg-muted-foreground/15",
};

/** Compact per-stage strip, CircleCI-style. */
function StageBar({ stages }: { stages: PipelineRunStage[] }) {
  return (
    <span className="flex gap-0.5" aria-hidden>
      {stages.map((s) => (
        <span
          key={s.name}
          title={`${stageLabel(s)}: ${STATUS_LABEL[s.status]}`}
          className={cn("h-2 w-5 rounded-sm", BAR_TONE[s.status])}
        />
      ))}
    </span>
  );
}
