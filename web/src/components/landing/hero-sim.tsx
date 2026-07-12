"use client";

import { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { Lock, TriangleAlert } from "lucide-react";
import { cn } from "@/lib/utils";

// A scripted, clearly-labelled simulation of one release earning its way
// through the ring pipeline. Two timelines: the happy path (loops forever)
// and a failure branch the visitor can arm, which shows the automatic
// rollback and then resumes the happy path at the retry.

const RINGS = [
  { key: "int", name: "Integration" },
  { key: "test", name: "Test" },
  { key: "acc", name: "Acceptance" },
  { key: "prod", name: "Production" },
] as const;

type RingState = "ok" | "deploy" | "verify" | "fail" | "locked";
type Tone = "ok" | "info" | "warn" | "err";

type Step = {
  d: number; // how long the step holds, ms
  chip: number; // station index the version chip sits at / glides to
  rings: RingState[];
  versions: string[];
  caption: string;
  capTone?: Tone;
  log?: string;
  tone?: Tone;
};

const STATE_HEX: Record<RingState, string> = {
  ok: "#22c55e",
  deploy: "#3b82f6",
  verify: "#3b82f6",
  fail: "#ef4444",
  locked: "#f59e0b",
};

const N = "v2.14.0"; // the release being promoted
const O = "v2.13.9"; // previous version in the middle rings
const P = "v2.13.2"; // what prod is running

const V_START = [N, O, O, P];
const V_TEST = [N, N, O, P];
const V_ACC = [N, N, N, P];
const V_PROD = [N, N, N, N];

const HAPPY: Step[] = [
  { d: 2600, chip: 0, rings: ["ok", "ok", "ok", "ok"], versions: V_START, caption: "payments-api · v2.14.0 healthy in int", log: "✓ int healthy · v2.14.0", tone: "ok" },
  { d: 1500, chip: 1, rings: ["ok", "deploy", "ok", "ok"], versions: V_TEST, caption: "promote int → test · deploying v2.14.0", log: "▸ promote int → test · kubectl set image", tone: "info" },
  { d: 900, chip: 1, rings: ["ok", "verify", "ok", "ok"], versions: V_TEST, caption: "health check test · attempt 1/3" },
  { d: 900, chip: 1, rings: ["ok", "verify", "ok", "ok"], versions: V_TEST, caption: "health check test · attempt 2/3" },
  { d: 1300, chip: 1, rings: ["ok", "ok", "ok", "ok"], versions: V_TEST, caption: "test healthy · promotion recorded", capTone: "ok", log: "✓ test healthy · promoted v2.14.0", tone: "ok" },
  // ── RESUME_AFTER_FAIL re-enters here ──
  { d: 1400, chip: 1, rings: ["ok", "ok", "ok", "ok"], versions: V_TEST, caption: "auto-promote engaged · test → acc", log: "▸ auto-promote test → acc", tone: "info" },
  { d: 1500, chip: 2, rings: ["ok", "ok", "deploy", "ok"], versions: V_ACC, caption: "deploying v2.14.0 to acc" },
  { d: 900, chip: 2, rings: ["ok", "ok", "verify", "ok"], versions: V_ACC, caption: "health check acc · attempt 1/3" },
  { d: 1300, chip: 2, rings: ["ok", "ok", "ok", "ok"], versions: V_ACC, caption: "acc healthy · promotion recorded", capTone: "ok", log: "✓ acc healthy · promoted v2.14.0", tone: "ok" },
  { d: 2200, chip: 2, rings: ["ok", "ok", "ok", "locked"], versions: V_ACC, caption: "prod is gated · production password required", capTone: "warn", log: "⚠ prod gated · awaiting password", tone: "warn" },
  { d: 1500, chip: 2, rings: ["ok", "ok", "ok", "locked"], versions: V_ACC, caption: "operator approved · promoting acc → prod", log: "▸ promote acc → prod · password accepted", tone: "info" },
  { d: 1500, chip: 3, rings: ["ok", "ok", "ok", "deploy"], versions: V_PROD, caption: "deploying v2.14.0 to prod" },
  { d: 1000, chip: 3, rings: ["ok", "ok", "ok", "verify"], versions: V_PROD, caption: "health check prod · attempt 1/3" },
  { d: 3600, chip: 3, rings: ["ok", "ok", "ok", "ok"], versions: V_PROD, caption: "v2.14.0 live in prod · every hop in history", capTone: "ok", log: "✓ v2.14.0 live in prod · history updated", tone: "ok" },
];

const RESUME_AFTER_FAIL = 5;

const FAIL: Step[] = [
  { d: 1300, chip: 1, rings: ["ok", "ok", "ok", "ok"], versions: V_TEST, caption: "auto-promote engaged · test → acc", log: "▸ auto-promote test → acc", tone: "info" },
  { d: 1500, chip: 2, rings: ["ok", "ok", "deploy", "ok"], versions: V_ACC, caption: "deploying v2.14.0 to acc" },
  { d: 900, chip: 2, rings: ["ok", "ok", "verify", "ok"], versions: V_ACC, caption: "health check acc · attempt 1/3 — 503", capTone: "err", log: "✗ acc /health · 503 (attempt 1/3)", tone: "err" },
  { d: 900, chip: 2, rings: ["ok", "ok", "verify", "ok"], versions: V_ACC, caption: "health check acc · attempt 2/3 — 503", capTone: "err", log: "✗ acc /health · 503 (attempt 2/3)", tone: "err" },
  { d: 1200, chip: 2, rings: ["ok", "ok", "fail", "ok"], versions: V_ACC, caption: "acc unhealthy after 3 attempts", capTone: "err", log: "✗ acc /health · 503 (attempt 3/3) — unhealthy", tone: "err" },
  { d: 1700, chip: 1, rings: ["ok", "ok", "deploy", "ok"], versions: V_TEST, caption: "rolling back acc → v2.13.9", capTone: "warn", log: "↩ rollback acc → v2.13.9", tone: "warn" },
  { d: 2800, chip: 1, rings: ["ok", "ok", "ok", "ok"], versions: V_TEST, caption: "failure contained · int and test untouched", capTone: "ok", log: "✓ history: promote acc failed · rolled_back=true", tone: "ok" },
];

type Line = { id: number; time: string; text: string; tone: Tone };

const TONE_TEXT: Record<Tone, string> = {
  ok: "text-emerald-400",
  info: "text-neutral-400",
  warn: "text-amber-400",
  err: "text-red-400",
};

function fmtClock(sec: number) {
  const h = Math.floor(sec / 3600) % 24;
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(h)}:${p(m)}:${p(s)}`;
}

export function HeroSim() {
  const reduce = useReducedMotion();
  const [pos, setPos] = useState({ mode: "happy" as "happy" | "fail", i: 0, cycle: 0 });
  const [ledger, setLedger] = useState<Line[]>([]);
  const lastLogKey = useRef("");
  const lineId = useRef(0);
  const clock = useRef(50527); // fake wall clock, starts at 14:02:07

  const script = pos.mode === "happy" ? HAPPY : FAIL;
  const step = script[pos.i];

  // Advance the timeline.
  useEffect(() => {
    const t = setTimeout(() => {
      setPos((p) => {
        const s = p.mode === "happy" ? HAPPY : FAIL;
        if (p.i + 1 < s.length) return { ...p, i: p.i + 1 };
        // Failure branch rejoins the happy path at the retry; the happy
        // path loops. cycle changes remount the chip so it fades in
        // instead of tweening back across the whole rail.
        return { mode: "happy", i: p.mode === "fail" ? RESUME_AFTER_FAIL : 0, cycle: p.cycle + 1 };
      });
    }, step.d);
    return () => clearTimeout(t);
  }, [pos, step.d]);

  // Append this step's ledger line exactly once (guards StrictMode).
  useEffect(() => {
    const key = `${pos.mode}:${pos.i}:${pos.cycle}`;
    if (!step.log || lastLogKey.current === key) return;
    lastLogKey.current = key;
    const time = fmtClock(clock.current);
    clock.current += Math.max(1, Math.round(step.d / 1000));
    setLedger((l) => [
      ...l.slice(-4),
      { id: lineId.current++, time, text: step.log!, tone: step.tone ?? "info" },
    ]);
  }, [pos, step]);

  const chipPct = (step.chip / (RINGS.length - 1)) * 100;
  const spring = reduce
    ? { duration: 0 }
    : { type: "spring" as const, stiffness: 60, damping: 16 };

  return (
    <div className="overflow-hidden rounded-2xl border border-white/10 bg-[#0b0b0c] shadow-[0_30px_80px_-40px_rgba(0,0,0,0.9)]">
      {/* Header */}
      <div className="flex items-center justify-between gap-3 border-b border-white/[0.07] px-4 py-2.5 sm:px-5">
        <div className="flex min-w-0 items-center gap-2.5 font-mono text-xs text-neutral-500">
          <span className="relative flex size-2">
            <span className="absolute inset-0 animate-ping rounded-full bg-emerald-500/60" />
            <span className="relative size-2 rounded-full bg-emerald-500" />
          </span>
          <span className="truncate">release-control · payments-api</span>
          <span className="hidden rounded border border-white/10 px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-neutral-500 sm:inline">
            simulated
          </span>
        </div>
        <button
          type="button"
          onClick={() => setPos((p) => (p.mode === "fail" ? p : { mode: "fail", i: 0, cycle: p.cycle }))}
          disabled={pos.mode === "fail"}
          className={cn(
            "flex shrink-0 items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs font-medium transition-colors",
            pos.mode === "fail"
              ? "cursor-default border-amber-500/25 text-amber-400/90"
              : "border-red-500/30 text-red-400 hover:bg-red-500/10",
          )}
        >
          <TriangleAlert aria-hidden className="size-3.5" />
          {pos.mode === "fail" ? "containing failure…" : "Fail the next health check"}
        </button>
      </div>

      {/* Rail */}
      <div className="relative px-8 pb-5 pt-14 sm:px-14 sm:pt-16">
        <div
          aria-hidden
          className="pointer-events-none absolute inset-0"
          style={{
            backgroundImage:
              "linear-gradient(rgba(255,255,255,0.025) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.025) 1px, transparent 1px)",
            backgroundSize: "32px 32px",
          }}
        />
        <div className="relative mx-4 h-[104px] sm:mx-6">
          {/* the line + progress */}
          <div className="absolute left-0 right-0 top-[28px] h-px bg-white/10" />
          <motion.div
            className="absolute left-0 top-[28px] h-px bg-emerald-500/50"
            animate={{ width: `${chipPct}%` }}
            transition={spring}
          />

          {/* stations */}
          {RINGS.map((r, i) => {
            const state = step.rings[i];
            const hex = STATE_HEX[state];
            const version = step.versions[i];
            return (
              <div
                key={r.key}
                className="absolute top-0 flex w-24 -translate-x-1/2 flex-col items-center"
                style={{ left: `${(i / (RINGS.length - 1)) * 100}%` }}
              >
                <div className="relative size-14">
                  {/* guide ring */}
                  <svg viewBox="0 0 56 56" className="absolute inset-0">
                    <circle cx="28" cy="28" r="26" fill="none" stroke="#ffffff" strokeOpacity="0.08" strokeWidth="1" strokeDasharray="1 5" />
                  </svg>
                  {/* status ring */}
                  <svg viewBox="0 0 56 56" className={cn("absolute inset-0", state === "verify" && "animate-pulse")}>
                    <circle
                      cx="28"
                      cy="28"
                      r="19"
                      fill="#0b0b0c"
                      stroke={hex}
                      strokeWidth="1.5"
                      strokeDasharray={state === "locked" ? "3 4" : undefined}
                      style={{ filter: state === "fail" || state === "ok" ? `drop-shadow(0 0 6px ${hex}55)` : undefined }}
                    />
                    <circle cx="28" cy="28" r="3.5" fill={hex} />
                  </svg>
                  {/* deploy spinner */}
                  {(state === "deploy" || state === "verify") && (
                    <svg viewBox="0 0 56 56" className="absolute inset-0 animate-spin [animation-duration:2.4s]">
                      <circle cx="28" cy="28" r="26" fill="none" stroke={hex} strokeWidth="1.5" strokeLinecap="round" strokeDasharray="26 138" />
                    </svg>
                  )}
                  {state === "locked" && (
                    <span className="absolute -right-1 -top-1 flex size-5 items-center justify-center rounded-full border border-amber-500/40 bg-[#0b0b0c]">
                      <Lock aria-hidden className="size-2.5 text-amber-400" />
                    </span>
                  )}
                </div>
                <p className="mt-2 font-mono text-xs font-medium text-neutral-200">{r.key}</p>
                <p className="text-[10px] text-neutral-600">{r.name}</p>
                <p
                  className={cn(
                    "mt-1 font-mono text-[10px]",
                    state === "deploy" ? "text-sky-400" : state === "fail" ? "text-red-400" : "text-neutral-500",
                  )}
                >
                  {version}
                </p>
              </div>
            );
          })}

          {/* travelling version chip */}
          <motion.div
            key={pos.cycle}
            className="absolute top-[28px] z-10 -translate-x-1/2 -translate-y-1/2"
            initial={{ opacity: 0, left: `${chipPct}%` }}
            animate={{ opacity: 1, left: `${chipPct}%` }}
            transition={{ left: spring, opacity: { duration: reduce ? 0 : 0.5 } }}
          >
            <span className="absolute -top-9 left-1/2 -translate-x-1/2 whitespace-nowrap rounded-md border border-white/15 bg-white/[0.07] px-1.5 py-0.5 font-mono text-[10px] text-neutral-100 shadow-lg backdrop-blur-sm">
              {N}
            </span>
            <span className="absolute -top-[13px] left-1/2 h-[7px] w-px -translate-x-1/2 bg-white/25" />
            <span className="block size-2.5 rounded-full bg-neutral-100 shadow-[0_0_8px_rgba(255,255,255,0.7)]" />
          </motion.div>
        </div>

        {/* caption */}
        <p
          aria-live="polite"
          className={cn(
            "relative mt-4 truncate text-center font-mono text-xs",
            TONE_TEXT[step.capTone ?? "info"],
          )}
        >
          <span className="text-neutral-600">▸ </span>
          {step.caption}
        </p>
      </div>

      {/* Ledger */}
      <div className="border-t border-white/[0.07] bg-black/40 px-4 py-3 sm:px-5">
        <p className="mb-1.5 font-mono text-[10px] uppercase tracking-widest text-neutral-600">
          audit history
        </p>
        <ul className="h-[95px] space-y-[3px] overflow-hidden font-mono text-[11px] leading-[19px]">
          <AnimatePresence initial={false}>
            {ledger.map((l) => (
              <motion.li
                key={l.id}
                initial={reduce ? false : { opacity: 0, y: 5 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.3 }}
                className="flex gap-3 whitespace-nowrap"
              >
                <span className="text-neutral-600">{l.time}</span>
                <span className={cn("truncate", TONE_TEXT[l.tone])}>{l.text}</span>
              </motion.li>
            ))}
          </AnimatePresence>
        </ul>
      </div>
    </div>
  );
}
