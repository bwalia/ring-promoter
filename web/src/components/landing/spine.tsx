"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { gsap, ScrollTrigger, useGSAP } from "./gsap/setup";
import { cn } from "@/lib/utils";

// The page spine: a release record running down the left gutter (xl+ only).
// The line draws with scroll and a node commits (ticks green) as each
// section's proof is passed — scrolling the page promotes you to prod.
// Purely presentational (aria-hidden); it appears only after client-side
// measurement, so no-JS readers simply never see it.

const STOPS = [
  { id: "protocol", label: "protocol" },
  { id: "live-ops", label: "live ops" },
  { id: "gate", label: "prod gate" },
  { id: "deployers", label: "deployers" },
  { id: "config", label: "config" },
  { id: "roadmap", label: "roadmap" },
  { id: "cta", label: "prod" },
] as const;

export function Spine() {
  const ref = useRef<HTMLDivElement>(null);
  const lineRef = useRef<HTMLDivElement>(null);
  const [ys, setYs] = useState<number[] | null>(null);
  const [reached, setReached] = useState(0);

  const measure = useCallback(() => {
    const wrap = ref.current?.parentElement;
    if (!wrap) return;
    const wrapTop = wrap.getBoundingClientRect().top;
    const next: number[] = [];
    for (const s of STOPS) {
      const el = document.getElementById(s.id);
      if (!el) return; // a section is missing — stay hidden rather than misalign
      next.push(el.getBoundingClientRect().top - wrapTop + 12);
    }
    setYs((prev) =>
      prev && prev.length === next.length && prev.every((v, i) => Math.abs(v - next[i]) < 1)
        ? prev
        : next,
    );
  }, []);

  useEffect(() => {
    // Measure after layout settles; fonts and FAQ toggles fire
    // ScrollTrigger.refresh (see scroll-refresh.tsx), so re-measure with it.
    const raf = requestAnimationFrame(measure);
    ScrollTrigger.addEventListener("refresh", measure);
    return () => {
      cancelAnimationFrame(raf);
      ScrollTrigger.removeEventListener("refresh", measure);
    };
  }, [measure]);

  useGSAP(
    () => {
      if (!ys || !lineRef.current) return;
      const mm = gsap.matchMedia();
      // The spine only exists at xl. The scrubbed line is a scroll progress
      // indicator (exempt from reduced-motion — it moves only with scroll).
      mm.add("(min-width: 1280px)", () => {
        STOPS.forEach((s, i) => {
          ScrollTrigger.create({
            trigger: `#${s.id}`,
            start: "top 55%",
            onEnter: () => setReached((r) => Math.max(r, i + 1)),
            onLeaveBack: () => setReached((r) => Math.min(r, i)),
          });
        });
        gsap.fromTo(
          lineRef.current,
          { scaleY: 0 },
          {
            scaleY: 1,
            ease: "none",
            scrollTrigger: {
              trigger: `#${STOPS[0].id}`,
              start: "top 55%",
              endTrigger: `#${STOPS[STOPS.length - 1].id}`,
              end: "top 55%",
              scrub: 0.6,
              refreshPriority: -1,
            },
          },
        );
      });
    },
    { dependencies: [ys], revertOnUpdate: true },
  );

  return (
    <div
      ref={ref}
      aria-hidden
      className="pointer-events-none absolute inset-y-0 z-10 hidden w-24 xl:block"
      style={{ left: "max(calc((100vw - 72rem) / 2 - 4.5rem), 16px)" }}
    >
      {ys && (
        <>
          {/* track + scroll-drawn progress */}
          <div
            className="absolute w-px bg-white/[0.07]"
            style={{ left: 3, top: ys[0], height: ys[ys.length - 1] - ys[0] }}
          />
          <div
            ref={lineRef}
            className="absolute w-px origin-top bg-emerald-500/60"
            style={{ left: 3, top: ys[0], height: ys[ys.length - 1] - ys[0] }}
          />
          {STOPS.map((s, i) => {
            const lit = reached > i;
            const isProd = i === STOPS.length - 1;
            return (
              <div
                key={s.id}
                className="absolute flex items-center gap-2"
                style={{ top: ys[i] - 3.5, left: 0 }}
              >
                <span
                  className={cn(
                    "size-[7px] rounded-full border transition-all duration-300",
                    lit
                      ? "border-emerald-400 bg-emerald-500 shadow-[0_0_6px_rgba(34,197,94,0.6)]"
                      : "border-neutral-700 bg-[#090909]",
                  )}
                />
                <span
                  className={cn(
                    "font-mono text-[10px] tracking-widest transition-colors duration-300",
                    lit ? (isProd ? "text-emerald-400" : "text-neutral-300") : "text-neutral-600",
                  )}
                >
                  {s.label}
                </span>
              </div>
            );
          })}
        </>
      )}
    </div>
  );
}
