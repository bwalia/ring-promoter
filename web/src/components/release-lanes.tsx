"use client";

import { Fragment } from "react";
import { STATUS_HEX, type NodeStatus } from "@/components/group-ring";
import {
  spokeSentence,
  type GroupSpan,
  type Spoke,
  type SpokeNode,
} from "@/lib/descent-layout";
import { cn } from "@/lib/utils";

const NODE_HEX: Record<SpokeNode["state"], string> = {
  off: STATUS_HEX.empty,
  empty: STATUS_HEX.empty,
  healthy: STATUS_HEX.healthy,
  failed: STATUS_HEX.failed,
};

/**
 * Lanes — the Descent stage's fallback for phones and large fleets: one row
 * per app, one column per promotion ring, Earth at the end of each row. Same
 * encoding as the orbit: solid pill = newest version, outline = an older one.
 */
export function ReleaseLanes({
  apps,
  spokes,
  ringOrder,
  spans,
  title,
  statusById,
  ttfbById,
  selected,
  onSelect,
  onOpen,
  onSeed,
  onOpenGroup,
  openLabel = "Open app",
}: {
  apps: string[];
  spokes: Map<string, Spoke>;
  ringOrder: string[];
  spans: GroupSpan[];
  title: (id: string) => string;
  statusById: Map<string, NodeStatus>;
  ttfbById: Map<string, number | null>;
  selected: string | null;
  onSelect: (id: string | null) => void;
  onOpen: (id: string) => void;
  onSeed?: (id: string) => void;
  onOpenGroup?: (id: string) => void;
  openLabel?: string;
}) {
  const cols = `minmax(7.5rem,1.3fr) repeat(${Math.max(1, ringOrder.length)}, minmax(0,1fr)) 1rem`;
  const spanAt = new Map(spans.map((s) => [s.from, s]));
  const firstUngrouped = spans.length ? spans[spans.length - 1].to + 1 : -1;

  return (
    <div
      aria-label="Release lanes"
      data-testid="release-lanes"
      className="font-mono text-[11px] tabular-nums"
    >
      <div
        aria-hidden
        className="sticky top-0 z-10 grid items-center gap-1.5 bg-[#07070a]/95 px-2 py-2 text-[10px] uppercase tracking-[0.12em] text-neutral-500 backdrop-blur-md"
        style={{ gridTemplateColumns: cols }}
      >
        <span>App</span>
        {ringOrder.map((r) => (
          <span key={r} className="text-center">
            {r}
          </span>
        ))}
        <span />
      </div>

      {apps.map((id, i) => {
        const spoke = spokes.get(id);
        const status = statusById.get(id) ?? "empty";
        const ttfb = ttfbById.get(id);
        const open = selected === id;
        const span = spanAt.get(i);
        return (
          <Fragment key={id}>
            {span && (
              <GroupHeader
                name={span.name}
                onOpen={onOpenGroup ? () => onOpenGroup(span.id) : undefined}
              />
            )}
            {i === firstUngrouped && <GroupHeader name="Ungrouped" />}
            <div className="border-t border-white/[0.05]">
              <button
                type="button"
                aria-expanded={open}
                aria-label={`${title(id)}: ${spoke ? spokeSentence(spoke) : "checking"}`}
                onClick={() => onSelect(open ? null : id)}
                className={cn(
                  "grid w-full items-center gap-1.5 px-2 py-2 text-left transition-colors",
                  open ? "bg-white/[0.08]" : "hover:bg-white/[0.04]",
                )}
                style={{ gridTemplateColumns: cols }}
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    aria-hidden
                    className="size-2 shrink-0 rounded-full"
                    style={{ background: STATUS_HEX[status] }}
                  />
                  <span className="min-w-0">
                    <span className="line-clamp-2 block break-words font-display text-[12.5px] font-medium leading-tight text-neutral-50">
                      {title(id)}
                    </span>
                    {ttfb != null && (
                      <span className="block text-[10px] text-neutral-500">
                        {Math.round(ttfb)}ms TTFB
                      </span>
                    )}
                  </span>
                </span>
                {ringOrder.map((r, ri) => (
                  <VersionPill key={r} node={spoke?.nodes[ri]} />
                ))}
                <span
                 
                  aria-hidden
                  className="size-3 justify-self-end rounded-full bg-[radial-gradient(circle_at_35%_35%,#7dd3fc,#1e3a5f_60%,#0b1a2e)] shadow-[0_0_8px_rgba(56,189,248,0.35)]"
                />
              </button>
              {open && spoke && (
                <div className="flex flex-wrap items-center justify-between gap-2 px-3 pb-3 pt-1">
                  <p className="min-w-0 font-sans text-xs text-neutral-300">
                    {spokeSentence(spoke)}
                  </p>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => onOpen(id)}
                      className="h-8 rounded-md bg-white px-3 font-sans text-xs font-medium text-neutral-900 transition-colors hover:bg-white/85"
                    >
                      {openLabel}
                    </button>
                    {onSeed && (
                      <button
                        type="button"
                        onClick={() => onSeed(id)}
                        className="h-8 rounded-md border border-white/15 px-3 font-sans text-xs font-medium text-neutral-100 transition-colors hover:bg-white/10"
                      >
                        Seed
                      </button>
                    )}
                  </div>
                </div>
              )}
            </div>
          </Fragment>
        );
      })}
    </div>
  );
}

function GroupHeader({ name, onOpen }: { name: string; onOpen?: () => void }) {
  return (
    <div className="px-2 pb-1 pt-3">
      <span>
        {onOpen ? (
          <button
            type="button"
            onClick={onOpen}
            className="font-sans text-[11px] font-medium text-neutral-400 underline-offset-2 hover:text-neutral-100 hover:underline"
          >
            {name}
          </button>
        ) : (
          <span className="font-sans text-[11px] font-medium text-neutral-500">{name}</span>
        )}
      </span>
    </div>
  );
}

function VersionPill({ node }: { node: SpokeNode | undefined }) {
  if (!node || node.state === "off") {
    return <span />;
  }
  if (!node.version) {
    return (
      <span
       
        className="flex h-6 items-center justify-center rounded-md border border-dashed border-white/10 text-neutral-600"
      >
        —
      </span>
    );
  }
  const hex = NODE_HEX[node.state];
  return (
    <span
     
      title={`${node.label}: ${node.version}${node.fresh ? "" : " (older version)"}${node.state === "failed" ? " · failing" : ""}${node.gateClosed ? ` · ${node.gateClosed}` : ""}`}
      className="flex h-6 min-w-0 items-center justify-center truncate rounded-md border px-1"
      style={
        node.fresh
          ? { background: `${hex}24`, color: hex, borderColor: `${hex}55` }
          : { color: hex, borderColor: `${hex}88` }
      }
    >
      {node.version}
    </span>
  );
}
