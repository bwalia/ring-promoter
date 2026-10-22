// Pure layout for the flow graphs (deploy run view, multi-service pipelines):
// a left-to-right DAG where a node's column is its longest-path depth from the
// roots, like a GitHub Actions workflow graph. No React here so it stays easy
// to reason about and cheap to recompute on every poll.

import type { GateState, JobStage } from "@/lib/types";

export type FlowStatus =
  | "queued"
  | "running"
  | "waiting"
  | "success"
  | "failed"
  | "skipped"
  | "cancelled"
  | "reused";

/**
 * task: a unit of work (build, deploy, health check)
 * gate: a Go / No-Go decision point (policy gates, QA agent verdict)
 * approval: a manual hold that waits for a person (CircleCI "hold" job)
 * trigger / terminal: the run's entry and exit points
 */
export type FlowNodeKind = "trigger" | "task" | "gate" | "approval" | "terminal";

/**
 * Which plane an edge belongs to. Data-plane edges carry an artifact or
 * version forward (build → deploy → next ring); control-plane edges carry a
 * decision (a gate's verdict, an approval) that lets the next node start.
 */
export type FlowPlane = "data" | "control";

export interface FlowChild {
  id: string;
  label: string;
  sublabel?: string;
  status: FlowStatus;
  startedAt?: string;
  finishedAt?: string;
  verdict?: GateState;
}

export interface FlowNode {
  id: string;
  kind: FlowNodeKind;
  label: string;
  sublabel?: string;
  status: FlowStatus;
  startedAt?: string;
  finishedAt?: string;
  /** Upstream node ids. Edges are drawn needs → this node. */
  needs: string[];
  /** Plane of the edges INTO this node (default: control into gates/approvals, data otherwise). */
  plane?: FlowPlane;
  /** Gate nodes: the Go / No-Go verdict. */
  verdict?: GateState;
  /** Parallel members (a stage's apps, a matrix) drawn inside one box. */
  children?: FlowChild[];
  /** Extra text for search (names that are not shown). */
  keywords?: string;
}

export interface PositionedNode extends FlowNode {
  x: number;
  y: number;
  w: number;
  h: number;
  column: number;
}

export interface PositionedEdge {
  id: string;
  from: string;
  to: string;
  plane: FlowPlane;
  /** SVG path from the source's right port to the target's left port. */
  d: string;
  state: EdgeState;
}

/**
 * idle: nothing has flowed yet
 * flowing: upstream is done and downstream is working — animate it
 * done: both ends finished successfully
 * blocked: downstream will not run because upstream failed / was rejected
 */
export type EdgeState = "idle" | "flowing" | "done" | "blocked";

export interface FlowLayout {
  nodes: PositionedNode[];
  edges: PositionedEdge[];
  width: number;
  height: number;
}

export const NODE_W = 232;
export const GATE_W = 168;
const TASK_H = 56;
const GATE_H = 72;
const CHILD_H = 30;
const BOX_HEAD = 44;
const COL_GAP = 72;
const ROW_GAP = 28;
const PAD = 24;
/** Group boxes show this many children before folding the rest. */
export const MAX_CHILDREN = 6;

export function nodeSize(
  n: FlowNode,
  visibleChildren: number,
): { w: number; h: number } {
  if (n.kind === "gate") {
    const rows = n.children?.length ? Math.min(visibleChildren, MAX_CHILDREN) : 0;
    return { w: GATE_W, h: GATE_H + rows * 22 };
  }
  if (n.kind === "trigger" || n.kind === "terminal") return { w: 176, h: TASK_H };
  // Approvals carry Approve / Reject buttons under the header.
  if (n.kind === "approval") return { w: NODE_W, h: TASK_H + 46 };
  if (n.children?.length) {
    const rows = Math.min(visibleChildren, MAX_CHILDREN);
    const more = visibleChildren > MAX_CHILDREN ? 1 : 0;
    return { w: NODE_W, h: BOX_HEAD + (rows + more) * CHILD_H + 10 };
  }
  return { w: NODE_W, h: TASK_H };
}

/** Longest-path depth of every node; unknown needs and cycles are ignored. */
export function columns(nodes: FlowNode[]): Map<string, number> {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const depth = new Map<string, number>();
  const visiting = new Set<string>();
  const visit = (id: string): number => {
    const known = depth.get(id);
    if (known !== undefined) return known;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const n = byId.get(id);
    let d = 0;
    for (const up of n?.needs ?? []) {
      if (byId.has(up)) d = Math.max(d, visit(up) + 1);
    }
    visiting.delete(id);
    depth.set(id, d);
    return d;
  };
  nodes.forEach((n) => visit(n.id));
  return depth;
}

export function edgeState(from: FlowStatus, to: FlowStatus): EdgeState {
  const ok = from === "success" || from === "reused";
  if (from === "failed" || from === "cancelled") return "blocked";
  if (to === "skipped" || to === "cancelled") return "blocked";
  if (ok && (to === "running" || to === "waiting")) return "flowing";
  if (ok && (to === "success" || to === "reused" || to === "failed")) return "done";
  return "idle";
}

// Gates render as a 44px square rotated 45°: its half-diagonal is ~31px.
const GATE_DIAMOND = 44;
const GATE_REACH = 31;

/** Where edges leave (right) and enter (left) a node. */
function port(n: PositionedNode, side: "in" | "out"): { x: number; y: number } {
  if (n.kind === "gate") {
    const cx = n.x + n.w / 2;
    return { x: side === "out" ? cx + GATE_REACH : cx - GATE_REACH, y: n.y + GATE_DIAMOND / 2 };
  }
  return { x: side === "out" ? n.x + n.w : n.x, y: n.y + Math.min(n.h, TASK_H) / 2 };
}

function edgePath(a: PositionedNode, b: PositionedNode): string {
  const { x: x1, y: y1 } = port(a, "out");
  const p2 = port(b, "in");
  const x2 = p2.x - 6; // leave room for the arrowhead
  const y2 = p2.y;
  const mid = (x1 + x2) / 2;
  return `M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`;
}

/**
 * Lay the graph out left to right. `visibleChildren` lets the caller shrink a
 * group box when a search filters its members.
 */
export function layoutFlow(
  nodes: FlowNode[],
  visibleChildren: (n: FlowNode) => number = (n) => n.children?.length ?? 0,
): FlowLayout {
  const depth = columns(nodes);
  const cols: FlowNode[][] = [];
  for (const n of nodes) {
    const c = depth.get(n.id) ?? 0;
    (cols[c] ??= []).push(n);
  }

  const positioned = new Map<string, PositionedNode>();
  let x = PAD;
  let height = 0;
  cols.forEach((col, ci) => {
    if (!col) return;
    const sizes = col.map((n) => nodeSize(n, visibleChildren(n)));
    const colW = Math.max(...sizes.map((s) => s.w));
    let y = PAD;
    col.forEach((n, i) => {
      const { w, h } = sizes[i];
      // Centre narrower nodes (gates) in a wide column.
      positioned.set(n.id, { ...n, x: x + (colW - w) / 2, y, w, h, column: ci });
      y += h + ROW_GAP;
    });
    height = Math.max(height, y - ROW_GAP + PAD);
    x += colW + COL_GAP;
  });

  const edges: PositionedEdge[] = [];
  for (const n of positioned.values()) {
    for (const up of n.needs) {
      const src = positioned.get(up);
      if (!src) continue;
      const plane =
        n.plane ??
        (n.kind === "gate" || n.kind === "approval" || src.kind === "gate" || src.kind === "approval"
          ? "control"
          : "data");
      edges.push({
        id: `${up}->${n.id}`,
        from: up,
        to: n.id,
        plane,
        d: edgePath(src, n),
        state: edgeState(src.status, n.status),
      });
    }
  }

  return {
    nodes: [...positioned.values()],
    edges,
    width: Math.max(x - COL_GAP + PAD, 0),
    height,
  };
}

// ---- GitHub Actions jobs → DAG ----

const ts = (iso?: string) => (iso ? new Date(iso).getTime() : NaN);

/**
 * The jobs API does not expose `needs:`, so infer the workflow graph from
 * timing: a job may depend on any job that had finished by the time it
 * started; keep only the nearest of those (drop any that is already an
 * ancestor of another candidate — a transitive reduction). Jobs that have not
 * started hang off every current sink. Exact for linear and fan-out/fan-in
 * shapes, and never leaves a finished job dangling.
 */
export function inferStageNeeds(stages: JobStage[]): Map<string, string[]> {
  const SLACK_MS = 2_000;
  const started = stages
    .filter((s) => !isNaN(ts(s.started_at)))
    .sort((a, b) => ts(a.started_at) - ts(b.started_at));
  const needs = new Map<string, string[]>();
  const ancestors = new Map<string, Set<string>>();

  for (const s of started) {
    const before = started.filter(
      (o) =>
        o.id !== s.id &&
        ancestors.has(o.id) &&
        !isNaN(ts(o.finished_at)) &&
        ts(o.finished_at) <= ts(s.started_at) + SLACK_MS,
    );
    const direct = before.filter(
      (o) => !before.some((p) => p.id !== o.id && ancestors.get(p.id)!.has(o.id)),
    );
    const anc = new Set<string>();
    for (const o of before) {
      anc.add(o.id);
      ancestors.get(o.id)!.forEach((a) => anc.add(a));
    }
    ancestors.set(s.id, anc);
    needs.set(s.id, direct.map((o) => o.id));
  }

  const upstream = new Set([...needs.values()].flat());
  const sinks = started.map((s) => s.id).filter((id) => !upstream.has(id));
  for (const s of stages) {
    if (!needs.has(s.id)) needs.set(s.id, sinks);
  }
  return needs;
}
