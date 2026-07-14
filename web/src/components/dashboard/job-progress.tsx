"use client";

import { useEffect, useState } from "react";
import {
  CheckCircle2,
  ChevronDown,
  CircleSlash,
  Loader2,
  Sparkles,
  X,
  XCircle,
} from "lucide-react";
import { ACTION_META } from "@/components/status";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { useActiveJob, useApps, useAppTitle, useDiagnoseJob } from "@/lib/queries";
import { duration } from "@/lib/time";
import type { Job, JobStep, StepStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

/**
 * Live view of the app's running (or just-finished) seed/promote/rollback:
 * one row per step with status, elapsed time and streaming logs, à la CI.
 */
export function JobProgress({ app }: { app: string }) {
  const { job, running, dismiss } = useActiveJob(app);
  const title = useAppTitle();

  // Tick every second while running so durations count up smoothly.
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!running) return;
    const t = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [running]);

  if (!job) return null;

  const finished = job.status === "success" || job.status === "failed";
  const meta = ACTION_META[job.action];

  return (
    <section
      className={cn(
        "overflow-hidden rounded-xl border bg-card",
        job.status === "failed" && "border-status-critical/50",
        job.status === "success" && "border-status-good/50",
      )}
      aria-live="polite"
    >
      <div className="flex items-center gap-3 border-b bg-muted/40 px-4 py-3">
        <JobStatusIcon status={job.status} />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">
            {meta?.label ?? job.action}
            <span className="text-muted-foreground"> · {title(job.app)}</span>
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {finished
              ? (job.result?.message ?? job.error ?? "finished")
              : "running — updates every second"}
          </p>
        </div>
        <span className="shrink-0 font-mono text-xs text-muted-foreground">
          {duration(job.started_at, job.finished_at)}
        </span>
        {finished && (
          <Button
            variant="ghost"
            size="icon"
            className="size-7 shrink-0"
            aria-label="Dismiss"
            onClick={dismiss}
          >
            <X aria-hidden className="size-4" />
          </Button>
        )}
      </div>

      <ol className="divide-y">
        {job.steps.length === 0 && (
          <li className="px-4 py-3 text-sm text-muted-foreground">
            Waiting for the first step…
          </li>
        )}
        {job.steps.map((step, i) => (
          <StepRow key={`${step.id}-${i}`} step={step} />
        ))}
      </ol>

      {job.status === "failed" && <DiagnosisFooter app={app} job={job} />}
    </section>
  );
}

/**
 * Footer of a FAILED job: a "Diagnose with AI" button that asks the server's
 * LLM to explain the failure in simple language, then the explanation itself.
 * Hidden entirely when the server has no AI diagnosis configured.
 */
function DiagnosisFooter({ app, job }: { app: string; job: Job }) {
  const { data: apps } = useApps();
  const diagnose = useDiagnoseJob(app, job.id);

  if (!apps?.ai_enabled) return null;

  // The generation runs server-side; "running" comes from the polled job, so
  // the state survives reloads and shows other users' in-flight diagnoses too.
  const running =
    diagnose.isPending || job.diagnosis_status === "running";
  const failed = !running && job.diagnosis_status === "failed";

  return (
    <div className="border-t bg-muted/20 px-4 py-3">
      {job.diagnosis ? (
        <div>
          <p className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Sparkles aria-hidden className="size-3.5" />
            AI diagnosis
          </p>
          <p className="text-sm leading-relaxed whitespace-pre-wrap">
            {job.diagnosis}
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {failed && (
            <p className="text-xs text-status-critical">
              Diagnosis failed: {job.diagnosis_error ?? "unknown error"}
            </p>
          )}
          <div className="flex items-center gap-3">
            <Button
              variant="outline"
              size="sm"
              onClick={() => diagnose.mutate()}
              disabled={running}
            >
              {running ? (
                <Loader2 aria-hidden className="size-4 animate-spin" />
              ) : (
                <Sparkles aria-hidden className="size-4" />
              )}
              {running
                ? "Analyzing failure…"
                : failed
                  ? "Try again"
                  : "Diagnose with AI"}
            </Button>
            {running && (
              <span className="text-xs text-muted-foreground">
                asking the model why this failed — can take a minute
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function JobStatusIcon({ status }: { status: Job["status"] }) {
  switch (status) {
    case "success":
      return (
        <CheckCircle2 aria-hidden className="size-5 text-status-good" />
      );
    case "failed":
      return <XCircle aria-hidden className="size-5 text-status-critical" />;
    default:
      return (
        <Loader2 aria-hidden className="size-5 animate-spin text-primary" />
      );
  }
}

function stepIcon(status: StepStatus) {
  switch (status) {
    case "running":
      return (
        <Loader2 aria-hidden className="size-4 animate-spin text-primary" />
      );
    case "success":
      return (
        <CheckCircle2 aria-hidden className="size-4 text-status-good" />
      );
    case "failed":
      return <XCircle aria-hidden className="size-4 text-status-critical" />;
    case "skipped":
      return (
        <CircleSlash aria-hidden className="size-4 text-muted-foreground" />
      );
  }
}

function StepRow({ step }: { step: JobStep }) {
  // Logs default open while a step runs or when it failed; successful steps
  // collapse to keep the panel scannable. A manual toggle wins from then on.
  const [override, setOverride] = useState<boolean | null>(null);
  const open =
    override ?? (step.status === "running" || step.status === "failed");
  const setOpen = (o: boolean) => setOverride(o);

  const hasLogs = (step.logs?.length ?? 0) > 0;

  return (
    <li>
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger
          className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-muted/40 disabled:cursor-default"
          disabled={!hasLogs}
        >
          {stepIcon(step.status)}
          <span className="min-w-0 flex-1 truncate text-sm">{step.title}</span>
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            {duration(step.started_at, step.finished_at)}
          </span>
          {hasLogs && (
            <ChevronDown
              aria-hidden
              className={cn(
                "size-4 shrink-0 text-muted-foreground transition-transform",
                open && "rotate-180",
              )}
            />
          )}
        </CollapsibleTrigger>
        {hasLogs && (
          <CollapsibleContent>
            <pre className="mx-4 mb-3 max-h-48 overflow-auto rounded-md bg-muted p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">
              {(step.logs ?? []).join("\n")}
            </pre>
          </CollapsibleContent>
        )}
      </Collapsible>
    </li>
  );
}
