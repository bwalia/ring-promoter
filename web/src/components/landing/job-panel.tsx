"use client";

import { useEffect, useRef, useState } from "react";
import { useReducedMotion } from "motion/react";
import { cn } from "@/lib/utils";

// "Evidence arrives in order": the job panel runs itself exactly once when it
// scrolls into view — steps resolve sequentially, the active row's duration
// counts up, log lines land as their step produces them, and then the panel
// settles into stillness. Server markup (and reduced motion) is the finished
// job. Row and log heights are fixed so nothing reflows while it runs.

const STEPS = [
  { label: "acquire app lock", ms: 500, final: "0.0s", shown: 0.0 },
  { label: "source health check — int", ms: 800, final: "0.3s", shown: 0.3 },
  { label: "deploy v2.14.0 to test", ms: 2400, final: "8.2s", shown: 8.2 },
  { label: "health check test — attempt 2/3", ms: 1700, final: "4.1s", shown: 4.1 },
  { label: "record history", ms: 500, final: "0.1s", shown: 0.1 },
] as const;

const LOGS = [
  { step: 2, delay: 100, text: "$ kubectl set image deploy/payments-api web=…:v2.14.0", bright: true },
  { step: 2, delay: 900, text: "deployment.apps/payments-api image updated", bright: false },
  { step: 2, delay: 1700, text: "waiting for rollout: 2/3 replicas updated…", bright: false },
  { step: 4, delay: 250, text: "✓ recorded: promote int → test · healthy", bright: true },
] as const;

const DONE = STEPS.length;

export function JobPanel() {
  const reduce = useReducedMotion();
  // SSR/no-JS/reduced-motion pose: the job has already completed.
  const [phase, setPhase] = useState<number>(DONE);
  const [logCount, setLogCount] = useState<number>(LOGS.length);
  const [elapsed, setElapsed] = useState({ phase: -1, ms: 0 });
  const [started, setStarted] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  // Arm: rewind to "queued" (idempotent, so StrictMode's double-mount is
  // harmless) and wait for the viewport.
  useEffect(() => {
    if (reduce) return;
    const raf = requestAnimationFrame(() => {
      setPhase((p) => (p === DONE ? -1 : p));
      setLogCount((c) => (c === LOGS.length ? 0 : c));
    });
    const el = rootRef.current;
    if (!el) return () => cancelAnimationFrame(raf);
    const io = new IntersectionObserver(
      ([e]) => {
        if (e.isIntersecting) {
          setStarted(true);
          io.disconnect();
        }
      },
      { threshold: 0.45 },
    );
    io.observe(el);
    return () => {
      cancelAnimationFrame(raf);
      io.disconnect();
    };
  }, [reduce]);

  // The run itself: one pass, then stillness.
  useEffect(() => {
    if (!started) return;
    let cancelled = false;
    const timers: ReturnType<typeof setTimeout>[] = [];
    const startOf = (step: number) =>
      200 + STEPS.slice(0, step).reduce((a, s) => a + s.ms, 0);
    STEPS.forEach((_, i) => {
      timers.push(setTimeout(() => !cancelled && setPhase(i), startOf(i)));
    });
    LOGS.forEach((l, idx) => {
      timers.push(
        setTimeout(
          () => !cancelled && setLogCount((c) => Math.max(c, idx + 1)),
          startOf(l.step) + l.delay,
        ),
      );
    });
    timers.push(setTimeout(() => !cancelled && setPhase(DONE), startOf(STEPS.length)));
    return () => {
      cancelled = true;
      timers.forEach(clearTimeout);
    };
  }, [started]);

  // Duration counter for the running row (100ms resolution is plenty).
  useEffect(() => {
    if (phase < 0 || phase >= DONE) return;
    const started = Date.now();
    const iv = setInterval(() => setElapsed({ phase, ms: Date.now() - started }), 100);
    return () => clearInterval(iv);
  }, [phase]);

  const runningDur = (i: number) => {
    const s = STEPS[i];
    const ms = elapsed.phase === i ? elapsed.ms : 0;
    const scaled = Math.min((ms * s.shown) / s.ms, s.shown);
    return `${scaled.toFixed(1)}s`;
  };

  return (
    <div
      ref={rootRef}
      className="overflow-hidden rounded-2xl border border-white/10 bg-[#0f131e] shadow-[0_30px_80px_-40px_rgba(0,0,0,0.9)]"
    >
      <div className="flex items-center justify-between border-b border-white/[0.07] px-4 py-2.5 font-mono text-xs text-slate-500">
        <span>
          <span className="text-slate-300">PROMOTE</span> payments-api · int → test
        </span>
        <span>job 8f3a…c2</span>
      </div>
      <ul className="space-y-1 px-4 py-4 font-mono text-xs">
        {STEPS.map((s, i) => {
          const state = i < phase || phase === DONE ? "done" : i === phase ? "run" : "todo";
          return (
            <li key={s.label} className="flex h-7 items-center gap-3">
              {state === "done" ? (
                <span className="flex size-4 items-center justify-center rounded-full bg-[#8b83ff]/15 text-[10px] text-[#a9a3ff]">
                  ✓
                </span>
              ) : state === "run" ? (
                <span className="relative flex size-4 items-center justify-center">
                  <span className="absolute inset-0 animate-ping rounded-full bg-sky-500/40" />
                  <span className="relative size-2 rounded-full bg-sky-400" />
                </span>
              ) : (
                <span className="flex size-4 items-center justify-center">
                  <span className="size-2 rounded-full border border-slate-700" />
                </span>
              )}
              <span
                className={cn(
                  "flex-1 transition-colors duration-300",
                  state === "todo"
                    ? "text-slate-600"
                    : state === "run"
                      ? "text-sky-300"
                      : "text-slate-300",
                )}
              >
                {s.label}
              </span>
              <span className="text-slate-600 tabular-nums">
                {state === "done" ? s.final : state === "run" ? runningDur(i) : "—"}
              </span>
            </li>
          );
        })}
      </ul>
      <div className="h-[104px] border-t border-white/[0.07] bg-black/40 px-4 py-3.5 font-mono text-[11px] leading-relaxed">
        {LOGS.slice(0, logCount).map((l) => (
          <p
            key={l.text}
            className={cn(
              l.bright
                ? l.text.startsWith("✓")
                  ? "text-[#a9a3ff]"
                  : "text-slate-400"
                : "text-slate-500",
            )}
          >
            {l.text}
          </p>
        ))}
      </div>
    </div>
  );
}
