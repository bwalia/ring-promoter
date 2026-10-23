"use client";

import { ExternalLink } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import type {
  GateState,
  ScanKind,
  SecurityCheck,
  SecurityEvaluation,
  Severity,
  SeverityCounts,
} from "@/lib/types";
import { SEVERITIES } from "@/lib/types";
import { cn } from "@/lib/utils";

export const KIND_LABEL: Record<ScanKind, string> = {
  sast: "SAST · code",
  sca: "SCA · dependencies",
  secrets: "Secrets",
  container: "Container image",
  iac: "IaC · infrastructure",
  dast: "DAST · running app",
  license: "Licences",
};

export const kindLabel = (k: string) => KIND_LABEL[k as ScanKind] ?? k.toUpperCase();

const SEV_TONE: Record<Severity, string> = {
  critical: "border-status-critical/70 bg-status-critical/20 text-status-critical",
  high: "border-status-critical/40 bg-status-critical/10 text-status-critical",
  medium: "border-status-warning/50 bg-status-warning/10 text-status-warning",
  low: "border-border bg-muted text-muted-foreground",
};

const SEV_SHORT: Record<Severity, string> = { critical: "C", high: "H", medium: "M", low: "L" };

export function SeverityBadge({ severity, className }: { severity: Severity; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded border px-1.5 py-px font-mono text-[10px] font-semibold uppercase",
        SEV_TONE[severity],
        className,
      )}
    >
      {severity}
    </span>
  );
}

/** C/H/M/L counters; zero counts are shown faint so a clean scan reads clean. */
export function SeverityCountsRow({
  counts,
  className,
}: {
  counts: Partial<SeverityCounts> | undefined;
  className?: string;
}) {
  return (
    <span className={cn("inline-flex items-center gap-1", className)}>
      {SEVERITIES.map((s) => {
        const n = counts?.[s] ?? 0;
        return (
          <span
            key={s}
            title={`${n} ${s}`}
            className={cn(
              "inline-flex min-w-7 items-center justify-center rounded border px-1 font-mono text-[10px] tabular-nums",
              n > 0 ? SEV_TONE[s] : "border-transparent text-muted-foreground/50",
            )}
          >
            {n}
            {SEV_SHORT[s]}
          </span>
        );
      })}
    </span>
  );
}

/** "2C 1H" — the non-zero blocking severities, for tight spaces. */
export function countsMeta(c: Partial<SeverityCounts> | undefined): string {
  if (!c) return "";
  const parts = SEVERITIES.filter((s) => (c[s] ?? 0) > 0 && s !== "low").map(
    (s) => `${c[s]}${SEV_SHORT[s]}`,
  );
  return parts.join(" ");
}

export function evaluationToVerdict(v: SecurityEvaluation["verdict"] | undefined): GateState {
  return v === "go" ? "go" : v === "no_go" ? "no_go" : v === "pending" ? "check" : "unknown";
}

const CHECK_TONE: Record<SecurityCheck["status"], string> = {
  pass: "border-status-good/60 bg-status-good/15 text-status-good",
  fail: "border-status-critical/60 bg-status-critical/15 text-status-critical",
  missing: "border-status-warning/60 bg-status-warning/15 text-status-warning",
  stale: "border-status-warning/60 bg-status-warning/15 text-status-warning",
};

export function CheckStatusPill({ status }: { status: SecurityCheck["status"] }) {
  return (
    <span
      className={cn(
        "inline-flex w-14 justify-center rounded border py-px font-mono text-[10px] font-semibold uppercase",
        CHECK_TONE[status],
      )}
    >
      {status}
    </span>
  );
}

/** One row per scan kind: verdict, who scanned, how many findings, why. */
export function SecurityChecksList({
  checks,
  className,
}: {
  checks: SecurityCheck[] | undefined;
  className?: string;
}) {
  if (!checks?.length) {
    return <p className={cn("text-sm text-muted-foreground", className)}>No scans checked.</p>;
  }
  return (
    <ul className={cn("divide-y rounded-md border text-sm", className)}>
      {checks.map((c) => (
        <li key={c.kind} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
          <CheckStatusPill status={c.status} />
          <span className="min-w-36 flex-1 font-medium">{kindLabel(c.kind)}</span>
          {c.status !== "missing" && <SeverityCountsRow counts={c} />}
          <span className="w-full text-xs text-muted-foreground sm:w-auto sm:min-w-40 sm:text-right">
            {c.status === "missing"
              ? (c.detail ?? "no scan reported for this version")
              : [
                  c.scanners?.join(", "),
                  c.waived ? `${c.waived} waived` : null,
                  c.detail,
                ]
                  .filter(Boolean)
                  .join(" · ")}
            {c.scanned_at && (
              <>
                {" · "}
                <RelativeTime iso={c.scanned_at} />
              </>
            )}
          </span>
        </li>
      ))}
    </ul>
  );
}

export function ScanLink({ url }: { url?: string }) {
  if (!url) return null;
  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
    >
      Report <ExternalLink aria-hidden className="size-3" />
    </a>
  );
}
