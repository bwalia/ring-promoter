"use client";

import { useMemo, useState } from "react";
import { ChevronRight, Lock, LockOpen, Search, ShieldCheck, Trash2 } from "lucide-react";
import { GateGlyph, VerdictPill } from "@/components/flow/flow-graph";
import { RelativeTime } from "@/components/relative-time";
import {
  ScanLink,
  SecurityChecksList,
  SeverityBadge,
  SeverityCountsRow,
  evaluationToVerdict,
  kindLabel,
} from "@/components/security/security-checks";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ApiError } from "@/lib/api";
import { useCreateWaiver, useDeleteWaiver, useSecurity } from "@/lib/queries";
import { useNow } from "@/lib/use-now";
import type {
  SecurityEvaluation,
  SecurityFinding,
  SecurityPolicy,
  SecurityScan,
  SecurityWaiver,
} from "@/lib/types";
import { SEVERITIES } from "@/lib/types";
import { cn } from "@/lib/utils";

const SEV_RANK = Object.fromEntries(SEVERITIES.map((s, i) => [s, i])) as Record<string, number>;

/**
 * Security & compliance for one app: what the policy demands, whether each
 * ring's current version would pass it, the scans behind that verdict and the
 * risk acceptances (waivers) that are in force.
 */
export function SecurityPanel({ app }: { app: string }) {
  const { data, error, isPending } = useSecurity(app);
  const [picked, setPicked] = useState<string | null>(null);

  // A server without the security endpoints: say nothing.
  if (error instanceof ApiError && error.status === 404) return null;
  if (isPending || !data) return null;

  const versions = [...new Set(data.scans.map((s) => s.version))];
  const live = data.posture.find((p) => p.version)?.version;
  const version = picked ?? live ?? versions[0] ?? null;

  return (
    <section className="rounded-xl border bg-card" aria-labelledby="security-heading">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-3">
        <h2 id="security-heading" className="rp-heading-panel flex items-center gap-2">
          <ShieldCheck aria-hidden className="size-4 text-muted-foreground" />
          Security &amp; compliance
        </h2>
        {data.policy && <PolicySummary policy={data.policy} />}
      </div>

      <Posture posture={data.posture} selected={version} onSelect={setPicked} />

      {data.scans.length === 0 ? (
        <NoScans app={app} />
      ) : (
        <Scans
          app={app}
          scans={data.scans}
          versions={versions}
          version={version}
          onVersion={setPicked}
          evaluation={data.posture.find((p) => p.version === version)}
        />
      )}

      <Waivers app={app} waivers={data.waivers} />
    </section>
  );
}

function PolicySummary({ policy }: { policy: SecurityPolicy }) {
  const max = SEVERITIES.filter((s) => policy.max[s] !== undefined)
    .map((s) => `${s} ≤ ${policy.max[s]}`)
    .join(", ");
  const source =
    policy.source === "app"
      ? "app policy"
      : policy.source === "default"
        ? "fleet default"
        : "built-in (pipelines only)";
  return (
    <p className="mr-auto flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
      <Badge variant="outline" className="text-[10px]">
        {source}
      </Badge>
      {policy.source !== "builtin" && (
        <span>
          enforced into <span className="font-mono">{policy.rings.join(", ")}</span>
        </span>
      )}
      <span>· requires {policy.require.map(kindLabel).join(", ") || "no scans"}</span>
      {max && <span>· {max}</span>}
      {!!policy.max_age_seconds && <span>· scans younger than {Math.round(policy.max_age_seconds / 3600)}h</span>}
    </p>
  );
}

/** One tile per ring: the version there and whether it passes the policy. */
function Posture({
  posture,
  selected,
  onSelect,
}: {
  posture: SecurityEvaluation[];
  selected: string | null;
  onSelect: (v: string) => void;
}) {
  return (
    <div className="grid grid-cols-2 gap-px border-b bg-border md:grid-cols-4">
      {posture.map((p) => {
        const v = evaluationToVerdict(p.verdict);
        const GuardIcon = p.guarded ? Lock : LockOpen;
        return (
          <button
            key={p.ring}
            type="button"
            disabled={!p.version}
            onClick={() => p.version && onSelect(p.version)}
            className={cn(
              "flex flex-col gap-1.5 bg-card px-4 py-3 text-left transition-colors hover:bg-muted/40 disabled:cursor-default disabled:hover:bg-card",
              p.version && p.version === selected && "bg-muted/50",
            )}
          >
            <span className="flex items-center gap-1.5">
              <GateGlyph gate="security" verdict={v} className="size-4" />
              <span className="font-mono text-sm font-medium">{p.ring}</span>
              <GuardIcon
                aria-label={p.guarded ? "enforced on promotion" : "not enforced"}
                className="size-3 text-muted-foreground"
              />
              <span className="ml-auto">
                {p.verdict === "none" ? (
                  <span className="text-[11px] text-muted-foreground">empty</span>
                ) : (
                  <VerdictPill verdict={v} />
                )}
              </span>
            </span>
            <span className="truncate font-mono text-xs text-muted-foreground">
              {p.version || "nothing deployed"}
            </span>
            {p.version && <SeverityCountsRow counts={p.counts} />}
            {p.detail && p.verdict !== "go" && (
              <span className="line-clamp-2 text-[11px] text-muted-foreground">{p.detail}</span>
            )}
          </button>
        );
      })}
    </div>
  );
}

function Scans({
  app,
  scans,
  versions,
  version,
  onVersion,
  evaluation,
}: {
  app: string;
  scans: SecurityScan[];
  versions: string[];
  version: string | null;
  onVersion: (v: string) => void;
  evaluation?: SecurityEvaluation;
}) {
  const [q, setQ] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const forVersion = scans.filter((s) => s.version === version);
  const needle = q.trim().toLowerCase();

  return (
    <div className="space-y-3 border-b p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-sm font-medium">Scans</h3>
        <div className="flex flex-wrap gap-1" role="radiogroup" aria-label="Version">
          {versions.slice(0, 8).map((v) => (
            <button
              key={v}
              type="button"
              role="radio"
              aria-checked={v === version}
              onClick={() => onVersion(v)}
              className={cn(
                "rounded-md border px-2 py-0.5 font-mono text-[11px] transition-colors",
                v === version ? "border-primary bg-primary/10" : "text-muted-foreground hover:bg-muted",
              )}
            >
              {v}
            </button>
          ))}
        </div>
        <div className="relative ml-auto">
          <Search
            aria-hidden
            className="absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="CVE, package, rule…"
            className="h-8 w-48 pl-7 text-sm md:w-60"
            aria-label="Search findings"
          />
        </div>
      </div>

      {evaluation?.checks && evaluation.checks.length > 0 && !needle && (
        <SecurityChecksList checks={evaluation.checks} />
      )}

      {forVersion.length === 0 ? (
        <p className="text-sm text-muted-foreground">No scans reported for {version}.</p>
      ) : (
        <ul className="divide-y rounded-md border">
          {forVersion.map((s) => {
            const findings = (s.findings ?? [])
              .filter((f) => !needle || findingText(f).includes(needle))
              .sort((a, b) => SEV_RANK[a.severity] - SEV_RANK[b.severity]);
            if (needle && findings.length === 0) return null;
            const expanded = open === s.id || !!needle;
            return (
              <li key={s.id}>
                <button
                  type="button"
                  onClick={() => setOpen(open === s.id ? null : s.id)}
                  aria-expanded={expanded}
                  className="flex w-full flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-left text-sm hover:bg-muted/40"
                >
                  <ChevronRight
                    aria-hidden
                    className={cn("size-3.5 text-muted-foreground transition-transform", expanded && "rotate-90")}
                  />
                  <span className="w-40 font-medium">{kindLabel(s.kind)}</span>
                  <span className="font-mono text-xs text-muted-foreground">{s.scanner}</span>
                  <SeverityCountsRow counts={s} className="ml-auto" />
                  <span className="w-20 text-right text-xs text-muted-foreground">
                    <RelativeTime iso={s.scanned_at} />
                  </span>
                </button>
                {expanded && (
                  <div className="space-y-2 bg-muted/20 px-3 pb-3 pt-1">
                    <div className="flex items-center gap-3">
                      {s.summary && <p className="text-xs text-muted-foreground">{s.summary}</p>}
                      <span className="ml-auto">
                        <ScanLink url={s.url} />
                      </span>
                    </div>
                    {findings.length === 0 ? (
                      <p className="text-xs text-muted-foreground">
                        {s.critical + s.high + s.medium + s.low === 0
                          ? "Clean: no findings."
                          : "The scanner reported counts only, no individual findings."}
                      </p>
                    ) : (
                      <FindingsTable app={app} findings={findings} />
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

const findingText = (f: SecurityFinding) =>
  [f.id, f.title, f.package, f.location, f.severity].filter(Boolean).join(" ").toLowerCase();

const FINDINGS_SHOWN = 50;

function FindingsTable({ app, findings }: { app: string; findings: SecurityFinding[] }) {
  const [all, setAll] = useState(false);
  const shown = all ? findings : findings.slice(0, FINDINGS_SHOWN);
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs">
        <thead className="text-left text-muted-foreground">
          <tr>
            <th className="py-1 pr-3 font-medium">Severity</th>
            <th className="py-1 pr-3 font-medium">Finding</th>
            <th className="py-1 pr-3 font-medium">Where</th>
            <th className="py-1 pr-3 font-medium">Fix</th>
            <th className="py-1 font-medium" />
          </tr>
        </thead>
        <tbody className="divide-y">
          {shown.map((f, i) => (
            <tr key={`${f.id}-${i}`} className={cn(f.waived && "opacity-60")}>
              <td className="py-1.5 pr-3">
                <SeverityBadge severity={f.severity} />
              </td>
              <td className="max-w-80 py-1.5 pr-3">
                <span className="font-mono">{f.id}</span>
                {f.title && <span className="block truncate text-muted-foreground">{f.title}</span>}
              </td>
              <td className="max-w-56 truncate py-1.5 pr-3 font-mono text-muted-foreground">
                {f.package ? `${f.package}${f.installed ? `@${f.installed}` : ""}` : f.location}
              </td>
              <td className="py-1.5 pr-3 font-mono">{f.fixed ?? "—"}</td>
              <td className="py-1.5 text-right">
                {f.waived ? (
                  <Badge variant="outline" className="text-[10px]">
                    waived
                  </Badge>
                ) : (
                  <WaiveButton app={app} finding={f.id} />
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {findings.length > shown.length && (
        <Button variant="ghost" size="sm" className="mt-1 h-7 text-xs" onClick={() => setAll(true)}>
          Show all {findings.length}
        </Button>
      )}
    </div>
  );
}

/** Accept the risk of one finding for a while, with a reason on the record. */
function WaiveButton({ app, finding }: { app: string; finding: string }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [days, setDays] = useState("30");
  const [ring, setRing] = useState("");
  const create = useCreateWaiver(app);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="sm" className="h-6 px-2 text-[11px]">
          Waive
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 space-y-3">
        <div>
          <p className="text-sm font-medium">Accept risk · {finding}</p>
          <p className="text-xs text-muted-foreground">
            A waiver is time-boxed and recorded in the audit log. The finding stops counting against
            the security gate until it expires.
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={`reason-${finding}`}>Reason</Label>
          <Input
            id={`reason-${finding}`}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Not reachable: the vulnerable path is unused"
            className="h-8 text-sm"
          />
        </div>
        <div className="flex gap-2">
          <div className="flex-1 space-y-1.5">
            <Label htmlFor={`days-${finding}`}>Days (max 90)</Label>
            <Input
              id={`days-${finding}`}
              type="number"
              min={1}
              max={90}
              value={days}
              onChange={(e) => setDays(e.target.value)}
              className="h-8 text-sm"
            />
          </div>
          <div className="flex-1 space-y-1.5">
            <Label htmlFor={`ring-${finding}`}>Ring (blank = all)</Label>
            <Input
              id={`ring-${finding}`}
              value={ring}
              onChange={(e) => setRing(e.target.value.trim())}
              placeholder="prod"
              className="h-8 font-mono text-sm"
            />
          </div>
        </div>
        <Button
          size="sm"
          className="w-full"
          disabled={!reason.trim() || create.isPending}
          onClick={() =>
            create.mutate(
              {
                finding,
                reason: reason.trim(),
                days: Math.min(90, Math.max(1, Number(days) || 30)),
                ...(ring ? { ring } : {}),
              },
              { onSuccess: () => setOpen(false) },
            )
          }
        >
          Record waiver
        </Button>
      </PopoverContent>
    </Popover>
  );
}

function Waivers({ app, waivers }: { app: string; waivers: SecurityWaiver[] }) {
  const del = useDeleteWaiver(app);
  const now = useNow(60_000);
  const sorted = useMemo(
    () =>
      [...waivers].sort(
        (a, b) =>
          Number(new Date(a.expires_at).getTime() < now) - Number(new Date(b.expires_at).getTime() < now),
      ),
    [waivers, now],
  );
  if (waivers.length === 0) return null;
  return (
    <div className="space-y-2 p-4">
      <h3 className="text-sm font-medium">Waivers</h3>
      <ul className="divide-y rounded-md border text-sm">
        {sorted.map((w) => {
          const expired = new Date(w.expires_at).getTime() < now;
          return (
            <li
              key={w.id}
              className={cn("flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2", expired && "opacity-50")}
            >
              <span className="font-mono text-xs">{w.finding}</span>
              <Badge variant="outline" className="font-mono text-[10px]">
                {w.ring || "all rings"}
              </Badge>
              <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground" title={w.reason}>
                {w.reason}
                {w.created_by && ` — ${w.created_by}`}
              </span>
              <span className="text-xs text-muted-foreground">
                {expired ? "expired " : "expires "}
                <RelativeTime iso={w.expires_at} />
              </span>
              {!expired && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  aria-label={`Revoke waiver for ${w.finding}`}
                  disabled={del.isPending}
                  onClick={() => del.mutate(w.id)}
                >
                  <Trash2 aria-hidden className="size-3.5" />
                </Button>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function NoScans({ app }: { app: string }) {
  return (
    <div className="space-y-2 border-b p-4 text-sm">
      <p className="text-muted-foreground">
        No scans reported yet. Post SARIF from your CI scanners (Trivy, Semgrep, Gitleaks, CodeQL,
        Checkov…) and the security gate checks every version against the policy before it moves on:
      </p>
      <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs leading-relaxed">
        {`jq -n --slurpfile s trivy.sarif \\
  '{app:"${app}", version:"'$TAG'", kind:"sca", sarif:$s[0]}' |
curl -fsS -X POST "$RP_URL/api/security/scans" \\
  -H "Authorization: Bearer $RP_TOKEN" -H 'Content-Type: application/json' -d @-`}
      </pre>
    </div>
  );
}
