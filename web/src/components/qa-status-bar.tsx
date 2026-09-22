"use client";

import { ExternalLink } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { useApps, useQAStatus } from "@/lib/queries";
import type { GateState, QAReport } from "@/lib/types";
import { cn } from "@/lib/utils";

type EnvState = "up" | "down" | "unknown";

const WORKFLOW_LABEL: Record<GateState, string> = {
  go: "GO",
  check: "CHECK",
  no_go: "NO-GO",
  unknown: "—",
};

const WORKFLOW_TONE: Record<GateState, string> = {
  go: "text-status-good",
  check: "text-status-warning",
  no_go: "text-status-critical",
  unknown: "text-muted-foreground",
};

const ENV_LABEL: Record<EnvState, string> = {
  up: "up",
  down: "down",
  unknown: "—",
};

const ENV_TONE: Record<EnvState, string> = {
  up: "text-status-good",
  down: "text-status-critical",
  unknown: "text-muted-foreground",
};

function summarizeWorkflow(reports: QAReport[]): GateState {
  if (reports.length === 0) return "unknown";
  if (reports.some((r) => r.workflow_verdict === "no_go")) return "no_go";
  if (reports.some((r) => r.workflow_verdict === "check")) return "check";
  if (reports.every((r) => r.workflow_verdict === "go")) return "go";
  if (reports.some((r) => r.workflow_verdict === "go")) return "check";
  return "unknown";
}

function summarizeEnv(reports: QAReport[]): EnvState {
  const known = reports.filter((r) => typeof r.env_healthy === "boolean");
  if (known.length === 0) return "unknown";
  if (known.some((r) => r.env_healthy === false)) return "down";
  return "up";
}

/**
 * Minimal QA agent strip: workflow go/no-go + environment up/down.
 * Hidden when qa_agent is not configured. Shows “waiting” until the agent
 * posts its first report.
 */
export function QaStatusBar({
  app,
  className,
  compact,
}: {
  /** When set, only that app's reports are summarised. */
  app?: string | null;
  className?: string;
  /** Fleet overlay style (over dark stage). */
  compact?: boolean;
}) {
  const apps = useApps();
  const qa = useQAStatus(app);

  if (!apps.data?.qa_enabled) return null;
  if (qa.isPending && !qa.data) return null;
  if (qa.data && !qa.data.enabled) return null;

  const reports = qa.data?.reports ?? [];
  const workflow = summarizeWorkflow(reports);
  const env = summarizeEnv(reports);
  const latest = reports[0];
  const name = qa.data?.name || "QA";
  const url = qa.data?.url;
  const waiting = reports.length === 0;

  return (
    <div
      className={cn(
        "inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1 text-xs",
        compact
          ? "rounded-md border border-white/15 bg-[#07070a]/70 px-3 py-1.5 text-neutral-200 backdrop-blur-md"
          : "text-muted-foreground",
        className,
      )}
      role="status"
      aria-label={`${name} status`}
    >
      <span className={cn("font-medium", compact ? "text-neutral-100" : "text-foreground")}>
        {name}
      </span>
      {waiting ? (
        <span className="text-muted-foreground">waiting for reports</span>
      ) : (
        <>
          <span>
            workflow{" "}
            <span className={cn("font-semibold", WORKFLOW_TONE[workflow])}>
              {WORKFLOW_LABEL[workflow]}
            </span>
          </span>
          <span className={compact ? "text-neutral-500" : "text-border"} aria-hidden>
            ·
          </span>
          <span>
            env{" "}
            <span className={cn("font-semibold", ENV_TONE[env])}>
              {ENV_LABEL[env]}
            </span>
          </span>
          {latest?.summary && (
            <>
              <span className={compact ? "text-neutral-500" : "text-border"} aria-hidden>
                ·
              </span>
              <span className="min-w-0 truncate" title={latest.summary}>
                {latest.summary}
              </span>
            </>
          )}
          {latest?.checked_at && (
            <RelativeTime
              iso={latest.checked_at}
              className={cn(
                "shrink-0",
                compact ? "text-neutral-500" : "text-muted-foreground",
              )}
            />
          )}
        </>
      )}
      {url && (
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className={cn(
            "inline-flex items-center gap-1 hover:underline",
            compact ? "text-neutral-300" : "text-foreground",
          )}
        >
          open
          <ExternalLink aria-hidden className="size-3" />
        </a>
      )}
    </div>
  );
}
