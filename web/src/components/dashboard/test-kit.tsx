"use client";

import { useState } from "react";
import {
  BookOpen,
  Braces,
  ChartLine,
  Check,
  ChevronDown,
  Copy,
  ExternalLink,
  Globe,
  Link2,
  Loader2,
  Package,
  QrCode,
  RefreshCw,
  Smartphone,
  Sparkles,
  TabletSmartphone,
  Tag,
  Workflow,
  type LucideIcon,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { toast } from "sonner";
import { RelativeTime } from "@/components/relative-time";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useGenerateTestPlan, useTestKit } from "@/lib/queries";
import type { RingView, TestLink } from "@/lib/types";
import { cn } from "@/lib/utils";

// After a deploy, an operator's next question is "where do I try it?". These
// components answer it from the ring's test kit: links from config, the ring's
// health host, and what the deploy itself produced (workflow run, artifacts,
// release, URLs printed in its logs) — plus an optional AI test plan.

const KIND_ICON: Record<string, LucideIcon> = {
  web: Globe,
  ios: Smartphone,
  android: TabletSmartphone,
  api: Braces,
  docs: BookOpen,
  dashboard: ChartLine,
  release: Tag,
  artifact: Package,
  ci: Workflow,
};

/** Kinds a tester opens directly, as opposed to build outputs. */
const TRY_KINDS = new Set(["web", "ios", "android", "api", "docs", "dashboard"]);

function LinkIcon({ kind, className }: { kind: string; className?: string }) {
  const Icon = KIND_ICON[kind] ?? Link2;
  return <Icon aria-hidden className={cn("size-4 shrink-0", className)} />;
}

function hostOf(url: string): string {
  try {
    const u = new URL(url);
    return u.host + (u.pathname === "/" ? "" : u.pathname);
  } catch {
    return url;
  }
}

/**
 * Compact row on a ring card: an "Open" button for the primary web link and
 * icon buttons for the other things worth trying (TestFlight, docs, ...).
 * Clicks never bubble to the card, which would open the details sheet.
 */
export function RingCardLinks({ view }: { view: RingView }) {
  const links = (view.links ?? []).filter((l) => l.source !== "log");
  if (!view.current_version || links.length === 0) return null;

  const primary = links.find((l) => l.kind === "web");
  const others = links
    .filter((l) => l !== primary && (TRY_KINDS.has(l.kind) || l.kind === "release"))
    .slice(0, 3);

  return (
    <div
      className="flex items-center gap-1.5"
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => e.stopPropagation()}
    >
      {primary && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              asChild
              variant="outline"
              size="sm"
              className="h-7 min-w-0 flex-1 justify-start gap-1.5 px-2 text-xs"
            >
              <a href={primary.url} target="_blank" rel="noreferrer">
                <ExternalLink aria-hidden className="size-3.5 shrink-0" />
                <span className="truncate">Open</span>
              </a>
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {primary.label}: {hostOf(primary.url)}
          </TooltipContent>
        </Tooltip>
      )}
      {others.map((l) => (
        <Tooltip key={l.url}>
          <TooltipTrigger asChild>
            <Button asChild variant="ghost" size="icon" className="size-7">
              <a
                href={l.url}
                target="_blank"
                rel="noreferrer"
                aria-label={l.label}
              >
                <LinkIcon kind={l.kind} className="size-3.5" />
              </a>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{l.label}</TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}

function CopyButton({ url }: { url: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-7 shrink-0"
      aria-label="Copy link"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(url);
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        } catch {
          toast.error("Could not copy the link");
        }
      }}
    >
      {copied ? (
        <Check aria-hidden className="size-3.5 text-status-good" />
      ) : (
        <Copy aria-hidden className="size-3.5" />
      )}
    </Button>
  );
}

/** Kinds worth opening on a phone: offered as a QR code. */
const MOBILE_KINDS = new Set(["ios", "android", "web"]);

/**
 * QR code for a link, so a tester at a desk can open a TestFlight build or the
 * web UI on their phone. Drawn on white with a quiet zone in both themes —
 * phone cameras need the contrast.
 */
function QrButton({ link }: { link: TestLink }) {
  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-7 shrink-0"
              aria-label={`Show QR code for ${link.label}`}
            >
              <QrCode aria-hidden className="size-3.5" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>Open on your phone</TooltipContent>
      </Tooltip>
      <PopoverContent className="w-auto p-3" align="end">
        <div className="flex flex-col items-center gap-2">
          <div className="rounded-md bg-white p-2.5">
            <QRCodeSVG
              value={link.url}
              size={168}
              level="M"
              bgColor="#ffffff"
              fgColor="#000000"
              title={link.url}
            />
          </div>
          <p className="max-w-[188px] text-center text-xs font-medium">
            {link.label}
          </p>
          <p className="max-w-[188px] text-center text-[11px] text-muted-foreground">
            Scan with your phone&apos;s camera
          </p>
        </div>
      </PopoverContent>
    </Popover>
  );
}

function LinkRow({ link, emphasis }: { link: TestLink; emphasis?: boolean }) {
  return (
    <li className="flex items-center gap-2">
      <a
        href={link.url}
        target="_blank"
        rel="noreferrer"
        className={cn(
          "group flex min-w-0 flex-1 items-center gap-2.5 rounded-md px-2 py-1.5 transition-colors hover:bg-muted",
          emphasis && "bg-muted/50",
        )}
      >
        <LinkIcon kind={link.kind} className="text-muted-foreground group-hover:text-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{link.label}</span>
          {link.why ? (
            <span className="block text-xs text-muted-foreground">{link.why}</span>
          ) : (
            <span className="block truncate font-mono text-[11px] text-muted-foreground">
              {hostOf(link.url)}
            </span>
          )}
        </span>
        <ExternalLink
          aria-hidden
          className="size-3.5 shrink-0 text-muted-foreground/0 transition-colors group-hover:text-muted-foreground"
        />
      </a>
      {MOBILE_KINDS.has(link.kind) && <QrButton link={link} />}
      <CopyButton url={link.url} />
    </li>
  );
}

/**
 * "Test this version" section of the ring details sheet: every link grouped
 * by what the tester does with it, and the AI test plan.
 */
export function TestKitSection({
  app,
  view,
  aiAvailable,
}: {
  app: string;
  view: RingView;
  aiAvailable: boolean;
}) {
  const ringName = view.ring.name;
  const version = view.current_version;
  const kit = useTestKit(app, ringName, version);
  const plan = useGenerateTestPlan(app, ringName, version);
  const [done, setDone] = useState<Record<number, boolean>>({});

  if (!version) return null;

  const links = kit.data?.links ?? view.links ?? [];
  const tryLinks = links.filter((l) => l.source !== "log" && TRY_KINDS.has(l.kind));
  const outputs = links.filter((l) => l.source !== "log" && !TRY_KINDS.has(l.kind));
  const fromLogs = links.filter((l) => l.source === "log");

  const status = kit.data?.plan_status ?? "none";
  const running = plan.isPending || status === "running";
  const testPlan = kit.data?.plan;

  return (
    <section className="space-y-3 rounded-lg border p-3">
      <div className="flex items-baseline justify-between gap-2">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Test this version
        </p>
        {kit.data?.captured_at && (
          <span className="text-[11px] text-muted-foreground">
            captured <RelativeTime iso={kit.data.captured_at} />
          </span>
        )}
      </div>

      {kit.isPending && !view.links ? (
        <Skeleton className="h-16 w-full" />
      ) : links.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          No links yet. Add <code>links</code> to this app&apos;s config, or
          give the ring an http(s) <code>health_url</code> to get an Open button.
        </p>
      ) : (
        <div className="space-y-3">
          {tryLinks.length > 0 && <LinkGroup title="Try it" links={tryLinks} />}
          {outputs.length > 0 && (
            <LinkGroup title="Build outputs" links={outputs} />
          )}
          {fromLogs.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="group flex w-full items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
                <ChevronDown
                  aria-hidden
                  className="size-3.5 transition-transform group-data-[state=closed]:-rotate-90"
                />
                Found in deploy logs ({fromLogs.length})
              </CollapsibleTrigger>
              <CollapsibleContent>
                <ul className="mt-1 space-y-0.5">
                  {fromLogs.map((l) => (
                    <LinkRow key={l.url} link={l} />
                  ))}
                </ul>
              </CollapsibleContent>
            </Collapsible>
          )}
        </div>
      )}

      {aiAvailable && (kit.data?.ai_enabled ?? true) && (
        <div className="space-y-2.5 border-t pt-3">
          {testPlan && !running ? (
            <>
              <div className="flex items-center justify-between gap-2">
                <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <Sparkles aria-hidden className="size-3.5" />
                  AI test plan
                </p>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={() => {
                    setDone({});
                    plan.mutate(true);
                  }}
                >
                  <RefreshCw aria-hidden className="size-3.5" />
                  Regenerate
                </Button>
              </div>
              {testPlan.summary && (
                <p className="text-sm leading-relaxed">{testPlan.summary}</p>
              )}
              {testPlan.checklist.length > 0 && (
                <ul className="space-y-1">
                  {testPlan.checklist.map((item, i) => (
                    <li key={i}>
                      <label className="flex cursor-pointer items-start gap-2 text-sm">
                        <input
                          type="checkbox"
                          className="mt-0.5 size-4 shrink-0 accent-[var(--status-good)]"
                          checked={!!done[i]}
                          onChange={(e) =>
                            setDone((d) => ({ ...d, [i]: e.target.checked }))
                          }
                        />
                        <span
                          className={cn(
                            done[i] && "text-muted-foreground line-through",
                          )}
                        >
                          {item}
                        </span>
                      </label>
                    </li>
                  ))}
                </ul>
              )}
              {testPlan.links.length > 0 && (
                <ul className="space-y-0.5">
                  {testPlan.links.map((l) => (
                    <LinkRow key={l.url} link={l} emphasis />
                  ))}
                </ul>
              )}
            </>
          ) : (
            <div className="space-y-2">
              {status === "failed" && !running && (
                <p className="text-xs text-status-critical">
                  Could not write a plan: {kit.data?.plan_error ?? "unknown error"}
                </p>
              )}
              <div className="flex flex-wrap items-center gap-3">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={running}
                  onClick={() => plan.mutate(false)}
                >
                  {running ? (
                    <Loader2 aria-hidden className="size-4 animate-spin" />
                  ) : (
                    <Sparkles aria-hidden className="size-4" />
                  )}
                  {running
                    ? "Writing a test plan…"
                    : status === "failed"
                      ? "Try again"
                      : "Suggest what to test"}
                </Button>
                <span className="text-xs text-muted-foreground">
                  {running
                    ? "reading the deploy's links and logs, can take a minute"
                    : "AI picks the links to open and a checklist"}
                </span>
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function LinkGroup({ title, links }: { title: string; links: TestLink[] }) {
  return (
    <div>
      <p className="mb-1 text-[11px] font-medium text-muted-foreground">{title}</p>
      <ul className="space-y-0.5">
        {links.map((l) => (
          <LinkRow key={l.url} link={l} />
        ))}
      </ul>
    </div>
  );
}

