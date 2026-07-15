"use client";

import { useRef } from "react";
import { gsap, useGSAP, MOTION_OK } from "./gsap/setup";
import { cn } from "@/lib/utils";

// "Permission has a boundary": eligibility energizes left→right through the
// auto gates and stops dead at the amber hold — the segment to prod never
// lights. Hovering the hold previews (dashed) what enabling it would open.
// Static markup is the final state: gates on, hold armed, prod unreached.

const HOPS = [
  { gate: "auto", on: true },
  { gate: "auto", on: true },
  { gate: "hold", on: false },
] as const;

const NODES = ["int", "test", "acc", "prod"] as const;

export function AutoPromoteDiagram() {
  const scope = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const q = gsap.utils.selector(scope);
      const mm = gsap.matchMedia();
      mm.add(MOTION_OK, () => {
        gsap.set(q(".ap-flow"), { scaleX: 0, transformOrigin: "left center" });
        gsap.set(q(".ap-gate"), { opacity: 0.35 });
        gsap.set(q(".ap-box"), { opacity: 0.4 });
        gsap.set(q(".ap-box-0"), { opacity: 1 }); // int is already healthy

        const tl = gsap.timeline({
          defaults: { ease: "commit" },
          scrollTrigger: { trigger: scope.current, start: "top 78%", once: true },
        });
        // hop 0: int → test
        tl.to(q(".ap-flow-0a"), { scaleX: 1, duration: 0.22, ease: "none" }, 0.1);
        tl.to(q(".ap-gate-0"), { opacity: 1, duration: 0.2 }, 0.3);
        tl.fromTo(q(".ap-gate-0"), { scale: 0.88 }, { scale: 1, duration: 0.25 }, 0.3);
        tl.to(q(".ap-flow-0b"), { scaleX: 1, duration: 0.22, ease: "none" }, 0.5);
        tl.to(q(".ap-box-1"), { opacity: 1, duration: 0.2 }, 0.68);
        // hop 1: test → acc
        tl.to(q(".ap-flow-1a"), { scaleX: 1, duration: 0.22, ease: "none" }, 0.8);
        tl.to(q(".ap-gate-1"), { opacity: 1, duration: 0.2 }, 1.0);
        tl.fromTo(q(".ap-gate-1"), { scale: 0.88 }, { scale: 1, duration: 0.25 }, 1.0);
        tl.to(q(".ap-flow-1b"), { scaleX: 1, duration: 0.22, ease: "none" }, 1.2);
        tl.to(q(".ap-box-2"), { opacity: 1, duration: 0.2 }, 1.38);
        // hop 2: the hold — flow reaches the gate and stops dead
        tl.to(q(".ap-flow-2a"), { scaleX: 1, duration: 0.22, ease: "none" }, 1.5);
        tl.to(q(".ap-gate-2"), { opacity: 1, duration: 0.15 }, 1.7);
        tl.fromTo(
          q(".ap-gate-2"),
          { scale: 1 },
          { scale: 0.9, duration: 0.1, ease: "contain" },
          1.72,
        );
        tl.to(q(".ap-gate-2"), { scale: 1, duration: 0.3 }, 1.84);
        tl.to(q(".ap-box-3"), { opacity: 0.7, duration: 0.3 }, 2.0); // prod: visible, not energized
      });
    },
    { scope },
  );

  return (
    <div
      ref={scope}
      className="overflow-x-auto rounded-xl border border-white/[0.07] bg-[#0a0c14] px-4 py-6 font-mono text-xs sm:px-5"
    >
      <div className="flex min-w-[360px] items-center justify-between gap-1">
        {NODES.map((ring, i) => (
          <div
            key={ring}
            className={cn(
              "flex flex-1 items-center gap-1 last:flex-none",
              i === 2 && "group/hold",
            )}
          >
            <span
              className={`ap-box ap-box-${i} rounded-md border border-white/15 bg-white/[0.05] px-1.5 py-1.5 text-slate-200 sm:px-2.5`}
            >
              {ring}
            </span>
            {i < 3 && (
              <span className="flex flex-1 items-center gap-1.5 px-1">
                <span className="relative h-px flex-1 bg-white/15">
                  <span className={`ap-flow ap-flow-${i}a absolute inset-0 bg-[#8b83ff]/60`} />
                </span>
                <span
                  className={cn(
                    `ap-gate ap-gate-${i} flex items-center gap-1 rounded-full border px-1.5 py-0.5 text-[10px]`,
                    HOPS[i].on
                      ? "border-[#8b83ff]/30 text-[#a9a3ff]"
                      : "cursor-help border-amber-500/30 text-amber-400",
                  )}
                  title={
                    HOPS[i].on
                      ? undefined
                      : "auto-promote into prod is off — enabling it is password-gated"
                  }
                >
                  <span
                    className={cn(
                      "size-1 rounded-full",
                      HOPS[i].on ? "bg-[#a9a3ff]" : "bg-amber-400",
                    )}
                  />
                  {HOPS[i].gate}
                </span>
                {i < 2 ? (
                  <span className="relative h-px flex-1 bg-white/15">
                    <span className={`ap-flow ap-flow-${i}b absolute inset-0 bg-[#8b83ff]/60`} />
                  </span>
                ) : (
                  // the un-energized route to prod: dashed, and only previewed
                  // (never lit) while hovering the hold gate
                  <span className="relative flex-1 border-t border-dashed border-white/15">
                    <span className="absolute -top-px left-0 w-full border-t border-dashed border-[#8b83ff]/50 opacity-0 transition-opacity duration-300 group-hover/hold:opacity-100" />
                  </span>
                )}
              </span>
            )}
          </div>
        ))}
      </div>
      <p className="mt-4 text-[11px] leading-relaxed text-slate-600">
        # a healthy int → test carries on to acc automatically;{" "}
        <span className="text-amber-500/80">acc holds for a human</span> before prod
      </p>
    </div>
  );
}
