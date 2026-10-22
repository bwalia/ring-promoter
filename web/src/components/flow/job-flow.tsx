"use client";

import { useMemo, useState } from "react";
import { ExternalLink } from "lucide-react";
import {
  FlowGraph,
  STATUS_LABEL,
  StatusIcon,
  VerdictPill,
  gateVerdict,
} from "@/components/flow/flow-graph";
import { inferStageNeeds, type FlowNode, type FlowStatus } from "@/lib/flow-layout";
import { useApps, useQAStatus } from "@/lib/queries";
import { duration } from "@/lib/time";
import type { GateState, Job, JobGate, JobStage, QAReport } from "@/lib/types";

const GATE_TITLE: Record<string, string> = {
  maintenance_window: "Maintenance window",
  qa_signoff: "QA sign-off",
  change_request: "Change request",
  grafana: "Grafana health",
};

const gateTitle = (g: string) =>
  GATE_TITLE[g] ?? g.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());

const gateToVerdict = (v: JobGate["verdict"]): GateState =>
  v === "pass" || v === "overridden" ? "go" : v === "fail" ? "no_go" : "unknown";

/** Target ring: reported by newer servers, else read off the deploy step title. */
export function jobRing(job: Job): string | undefined {
  if (job.ring) return job.ring;
  for (const s of job.steps) {
    const m = /\bto (\w+)$/.exec(s.title);
    if (s.id === "deploy" && m) return m[1];
  }
  return undefined;
}

function stageStatus(s: JobStage): FlowStatus {
  return s.status;
}

/** The QA agent's latest workflow verdict for app on ring, if any. */
function qaVerdictFor(reports: QAReport[] | undefined, app: string, ring?: string) {
  return reports?.find((r) => r.app === app && (r.ring ?? "") === (ring ?? ""));
}

/**
 * Turn one job into a run graph:
 *   trigger → policy gates ◆ → [steps, with a deploy step expanded into the
 *   CI workflow's own jobs] → QA gate ◆ → ring live
 */
export function buildJobFlow(
  job: Job,
  opts: { qaEnabled: boolean; qa?: QAReport },
): FlowNode[] {
  const ring = jobRing(job);
  const nodes: FlowNode[] = [];
  const finished = job.status === "success" || job.status === "failed";
  const version = job.result?.state?.current_version;

  nodes.push({
    id: "trigger",
    kind: "trigger",
    label: `${job.action[0].toUpperCase()}${job.action.slice(1)} requested`,
    sublabel: [job.app, ring && `→ ${ring}`].filter(Boolean).join(" "),
    status: job.status === "pending" ? "running" : "success",
    startedAt: job.started_at,
    needs: [],
  });
  let tail = ["trigger"];

  // Go / No-Go: the promotion policy evaluated before anything is deployed.
  if (job.action === "seed" || job.action === "promote") {
    const gates = job.gates ?? [];
    const failed = gates.some((g) => g.verdict === "fail");
    const anyStep = job.steps.length > 0;
    const status: FlowStatus = failed
      ? "failed"
      : gates.length || anyStep || finished
        ? "success"
        : "running";
    nodes.push({
      id: "policy",
      kind: "gate",
      label: "Policy gates",
      sublabel: gates.length
        ? `${gates.length} check${gates.length === 1 ? "" : "s"}`
        : status === "success"
          ? `no policy guards ${ring ?? "this ring"}`
          : "evaluating…",
      status,
      verdict: failed ? "no_go" : status === "success" ? "go" : "check",
      needs: tail,
      children: gates.map((g) => ({
        id: g.gate,
        label: gateTitle(g.gate),
        sublabel: g.detail,
        status: g.verdict === "fail" ? "failed" : "success",
        verdict: gateToVerdict(g.verdict),
      })),
    });
    tail = ["policy"];
  }

  job.steps.forEach((step, i) => {
    const id = `step-${i}`;
    const stages = step.stages ?? [];
    if (stages.length > 0) {
      // The CI workflow's own jobs, laid out like its Actions graph.
      const needs = inferStageNeeds(stages);
      const ids = new Set(stages.map((s) => s.id));
      const hasDownstream = new Set<string>();
      for (const s of stages) {
        const up = (needs.get(s.id) ?? []).filter((n) => ids.has(n));
        up.forEach((u) => hasDownstream.add(u));
        nodes.push({
          id: `${id}/${s.id}`,
          kind: "task",
          label: s.name,
          sublabel: step.title,
          status: stageStatus(s),
          startedAt: s.started_at,
          finishedAt: s.finished_at,
          needs: up.length ? up.map((u) => `${id}/${u}`) : tail,
          keywords: (s.steps ?? []).map((x) => x.name).join(" "),
        });
      }
      tail = stages.filter((s) => !hasDownstream.has(s.id)).map((s) => `${id}/${s.id}`);
      return;
    }
    nodes.push({
      id,
      kind: "task",
      label: step.title,
      status: step.status,
      startedAt: step.started_at,
      finishedAt: step.finished_at,
      needs: tail,
    });
    tail = [id];
  });

  if (!finished && job.steps.every((s) => s.status !== "running")) {
    nodes.push({ id: "next", kind: "task", label: "Starting next step…", status: "queued", needs: tail });
    tail = ["next"];
  }

  // QA agent Go / No-Go on the ring the version just reached.
  if (opts.qaEnabled && ring && job.action !== "rollback") {
    const v = opts.qa?.workflow_verdict ?? "unknown";
    const status: FlowStatus = !finished
      ? "queued"
      : job.status === "failed"
        ? "skipped"
        : v === "go"
          ? "success"
          : v === "no_go"
            ? "failed"
            : "waiting";
    nodes.push({
      id: "qa",
      kind: "gate",
      label: "QA gate",
      sublabel: opts.qa?.summary ?? (status === "waiting" ? "waiting for the QA agent" : `QA on ${ring}`),
      status,
      verdict: status === "queued" || status === "skipped" ? "unknown" : v,
      needs: tail,
    });
    tail = ["qa"];
  }

  nodes.push({
    id: "done",
    kind: "terminal",
    label:
      job.status === "failed"
        ? job.result?.rolled_back
          ? "Rolled back"
          : "Stopped"
        : ring
          ? `Live on ${ring}`
          : "Done",
    sublabel: version,
    status: job.status === "success" ? "success" : job.status === "failed" ? "failed" : "queued",
    finishedAt: job.finished_at,
    needs: tail,
  });

  return nodes;
}

/** Graph + detail panel for one job. */
export function JobFlow({ job }: { job: Job }) {
  const { data: apps } = useApps();
  const qa = useQAStatus(job.app);
  const ring = jobRing(job);
  const report = qaVerdictFor(qa.data?.reports, job.app, ring);
  const nodes = useMemo(
    () => buildJobFlow(job, { qaEnabled: !!apps?.qa_enabled, qa: report }),
    [job, apps?.qa_enabled, report],
  );
  const [selected, setSelected] = useState<string | null>(null);

  // Default focus: the live node, else the failure, else nothing.
  const focus =
    selected ??
    nodes.find((n) => n.status === "running" || n.status === "waiting")?.id ??
    nodes.find((n) => n.status === "failed")?.id ??
    null;

  return (
    <div className="space-y-3 p-3">
      <FlowGraph
        ariaLabel={`Run graph of ${job.action} ${job.app}`}
        nodes={nodes}
        selected={focus}
        onSelect={(id, child) => setSelected(child ? `${id}/${child}` : id)}
        className="pb-8"
      />
      {focus && <JobNodeDetail job={job} nodeId={focus} nodes={nodes} report={report} />}
    </div>
  );
}

function JobNodeDetail({
  job,
  nodeId,
  nodes,
  report,
}: {
  job: Job;
  nodeId: string;
  nodes: FlowNode[];
  report?: QAReport;
}) {
  // A step: its logs. A CI job: its steps and a link to the run.
  const stepMatch = /^step-(\d+)(?:\/(.+))?$/.exec(nodeId);
  if (stepMatch) {
    const step = job.steps[Number(stepMatch[1])];
    if (!step) return null;
    const stage = stepMatch[2]
      ? step.stages?.find((s) => s.id === stepMatch[2])
      : undefined;
    if (stage) {
      return (
        <Panel
          title={stage.name}
          status={stage.status}
          meta={stage.started_at ? duration(stage.started_at, stage.finished_at) : undefined}
          link={stage.url}
        >
          <ol className="divide-y text-sm">
            {(stage.steps ?? []).map((s) => (
              <li key={s.number} className="flex items-center gap-2 py-1.5">
                <StatusIcon status={s.status} className="size-3.5" />
                <span className="w-6 shrink-0 font-mono text-[11px] text-muted-foreground">
                  {s.number}
                </span>
                <span className="min-w-0 flex-1 truncate">{s.name}</span>
                {s.started_at && (
                  <span className="font-mono text-[11px] text-muted-foreground">
                    {duration(s.started_at, s.finished_at)}
                  </span>
                )}
              </li>
            ))}
            {!stage.steps?.length && (
              <li className="py-1.5 text-muted-foreground">No steps reported yet.</li>
            )}
          </ol>
          <Logs lines={step.logs} title={`${step.title} — log`} />
        </Panel>
      );
    }
    return (
      <Panel
        title={step.title}
        status={step.status}
        meta={duration(step.started_at, step.finished_at)}
      >
        <Logs lines={step.logs} />
      </Panel>
    );
  }

  const node = nodes.find((n) => n.id === nodeId.split("/")[0]);
  if (!node) return null;
  if (node.id === "policy") {
    return (
      <Panel title="Policy gates — Go / No-Go" status={node.status} verdict={gateVerdict(node)}>
        {(job.gates ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">{node.sublabel}</p>
        ) : (
          <ul className="space-y-1.5 text-sm">
            {(job.gates ?? []).map((g) => (
              <li key={g.gate} className="flex items-start gap-2">
                <VerdictPill verdict={gateToVerdict(g.verdict)} className="mt-0.5" />
                <span>
                  <span className="font-medium">{gateTitle(g.gate)}</span>
                  {g.detail && <span className="text-muted-foreground"> — {g.detail}</span>}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Panel>
    );
  }
  if (node.id === "qa") {
    return (
      <Panel title="QA gate — Go / No-Go" status={node.status} verdict={gateVerdict(node)}>
        <p className="text-sm">
          {report?.summary ?? "The QA agent has not reported on this ring yet."}
        </p>
        {report?.detail && (
          <p className="mt-1 text-xs text-muted-foreground">{report.detail}</p>
        )}
      </Panel>
    );
  }
  return (
    <Panel title={node.label} status={node.status}>
      <p className="text-sm text-muted-foreground">
        {node.sublabel ?? STATUS_LABEL[node.status]}
      </p>
      {node.id === "done" && (job.error || job.result?.message) && (
        <p className="mt-1 text-sm">{job.error ?? job.result?.message}</p>
      )}
    </Panel>
  );
}

function Panel({
  title,
  status,
  verdict,
  meta,
  link,
  children,
}: {
  title: string;
  status: FlowStatus;
  verdict?: GateState;
  meta?: string;
  link?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <StatusIcon status={status} />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{title}</span>
        {verdict && <VerdictPill verdict={verdict} />}
        {meta && <span className="font-mono text-xs text-muted-foreground">{meta}</span>}
        {link && (
          <a
            href={link}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
          >
            Open <ExternalLink aria-hidden className="size-3" />
          </a>
        )}
      </div>
      <div className="p-3">{children}</div>
    </div>
  );
}

function Logs({ lines, title }: { lines?: string[]; title?: string }) {
  if (!lines?.length) return null;
  return (
    <div className="mt-2">
      {title && <p className="mb-1 text-xs text-muted-foreground">{title}</p>}
      <pre className="max-h-64 overflow-auto rounded-md bg-muted p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">
        {lines.join("\n")}
      </pre>
    </div>
  );
}
