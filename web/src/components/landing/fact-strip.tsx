"use client";

import { useRef } from "react";
import { gsap, useGSAP, MOTION_OK } from "./gsap/setup";

// Facts are stamped into the record, not merely revealed: each one gets a
// single left→right verification sweep, settles to full contrast, and
// receives its tick. Static markup is the stamped (final) state.

const FACTS = [
  ["Single Go binary", "UI, API and promoter in one process"],
  ["Kubernetes + VMs", "kubectl and GitHub Actions, side by side"],
  ["Safe across replicas", "Postgres advisory locks serialize every op"],
  ["Onboard in YAML", "new apps are configuration, not code"],
] as const;

export function FactStrip() {
  const scope = useRef<HTMLElement>(null);

  useGSAP(
    () => {
      const q = gsap.utils.selector(scope);
      const mm = gsap.matchMedia();
      mm.add(MOTION_OK, () => {
        gsap.set(q(".fs-body"), { opacity: 0.35 });
        gsap.set(q(".fs-tick"), { opacity: 0, scale: 0.5 });
        gsap.set(q(".fs-sweep"), { xPercent: -110 });
        const tl = gsap.timeline({
          scrollTrigger: { trigger: scope.current, start: "top 82%", once: true },
        });
        FACTS.forEach((_, i) => {
          const at = i * 0.14;
          tl.to(q(`.fs-sweep-${i}`), { xPercent: 110, duration: 0.55, ease: "none" }, at);
          tl.to(q(`.fs-body-${i}`), { opacity: 1, duration: 0.3, ease: "commit" }, at + 0.25);
          tl.to(
            q(`.fs-tick-${i}`),
            { opacity: 1, scale: 1, duration: 0.25, ease: "commit" },
            at + 0.42,
          );
        });
      });
    },
    { scope },
  );

  return (
    <section ref={scope} className="border-y border-white/[0.07]">
      <div className="mx-auto grid max-w-6xl grid-cols-2 divide-white/[0.07] max-lg:gap-px lg:grid-cols-4 lg:divide-x">
        {FACTS.map(([title, sub], i) => (
          <div key={title} className="relative overflow-hidden px-4 py-6 sm:px-6">
            <div
              aria-hidden
              className={`fs-sweep fs-sweep-${i} pointer-events-none absolute inset-y-0 left-0 w-full bg-gradient-to-r from-transparent via-emerald-500/[0.08] to-transparent`}
            />
            <div className={`fs-body fs-body-${i}`}>
              <p className="flex items-center justify-between gap-2 font-mono text-[11px] uppercase tracking-widest text-neutral-500">
                {title}
                <span className={`fs-tick fs-tick-${i} text-emerald-500`} aria-hidden>
                  ✓
                </span>
              </p>
              <p className="mt-1.5 text-sm text-neutral-300">{sub}</p>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
