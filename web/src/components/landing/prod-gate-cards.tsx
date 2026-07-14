"use client";

import { useRef } from "react";
import { gsap, useGSAP, MOTION_OK } from "./gsap/setup";

// "Two directions, two rules." Left card: a version travels toward prod and
// is physically stopped at the lock, which clicks shut. Right card: one fast,
// uninterrupted reverse move — leaving prod has lower latency than entering.
// The demos run as one choreographed pair (left, then right) so the asymmetry
// is unmistakable. Static markup is both demos in their final pose.

const AM = "#f59e0b";
const EM = "#22c55e";

export function ProdGateCards() {
  const scope = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const q = gsap.utils.selector(scope);
      const mm = gsap.matchMedia();
      mm.add(MOTION_OK, () => {
        // rewind: chips back to their starting stations, shackle open
        gsap.set(q(".pg-chip-in"), { x: -132 });
        gsap.set(q(".pg-chip-out"), { x: 128 });
        gsap.set(q(".pg-shackle"), { y: -3.5 });
        gsap.set(q(".pg-tick-out"), { opacity: 0 });

        const tl = gsap.timeline({
          scrollTrigger: { trigger: scope.current, start: "top 75%", once: true },
        });

        // entering prod: travel, stop at the lock, the lock clicks shut
        tl.to(q(".pg-chip-in"), { x: 0, duration: 0.7, ease: "advance" }, 0.2);
        tl.to(
          q(".pg-chip-in"),
          { scaleX: 0.82, scaleY: 1.14, duration: 0.09, transformOrigin: "100% 50%" },
          0.88,
        );
        tl.to(q(".pg-chip-in"), { scaleX: 1, scaleY: 1, duration: 0.22, ease: "commit" }, 0.97);
        tl.to(q(".pg-shackle"), { y: 0, duration: 0.13, ease: "contain" }, 1.0);
        tl.fromTo(
          q(".pg-lock-body"),
          { strokeOpacity: 1 },
          { strokeOpacity: 0.55, duration: 0.5, ease: "commit", immediateRender: false },
          1.13,
        );

        // leaving prod: one decisive reverse move, then the confirmation tick
        tl.to(q(".pg-chip-out"), { x: 0, duration: 0.32, ease: "contain" }, 1.5);
        tl.to(q(".pg-tick-out"), { opacity: 1, duration: 0.15 }, 1.82);
        tl.fromTo(
          q(".pg-tick-out"),
          { strokeDasharray: 1, strokeDashoffset: 1 },
          { strokeDashoffset: 0, duration: 0.2, ease: "none", immediateRender: false },
          1.82,
        );
      });
    },
    { scope },
  );

  return (
    <div ref={scope} className="mx-auto mt-12 grid max-w-4xl grid-cols-1 gap-5 sm:grid-cols-2">
      {/* entering prod */}
      <div className="h-full rounded-xl border border-amber-500/20 bg-[#0a0c14] p-6">
        <svg viewBox="0 0 220 44" className="h-10 w-full" aria-hidden>
          <line x1="10" y1="26" x2="168" y2="26" stroke="#ffffff" strokeOpacity="0.12" strokeWidth="1.5" />
          <text x="10" y="12" fontSize="9" fill="#737373" fontFamily="var(--font-geist-mono), monospace">
            acc
          </text>
          <text x="176" y="12" fontSize="9" fill={AM} textAnchor="end" fontFamily="var(--font-geist-mono), monospace">
            prod
          </text>
          {/* the lock */}
          <g className="pg-lock-body" stroke={AM} strokeWidth="1.5" fill="none">
            <rect x="184" y="22" width="18" height="14" rx="2.5" />
          </g>
          <path
            className="pg-shackle"
            d="M 188 22 v -5 a 5 5 0 0 1 10 0 v 5"
            stroke={AM}
            strokeWidth="1.5"
            fill="none"
          />
          {/* the version, stopped at the gate */}
          <circle className="pg-chip-in" cx="172" cy="26" r="5" fill="#f5f5f5" />
        </svg>
        <h3 className="mt-4 font-semibold text-slate-100">Entering prod asks for the password</h3>
        <p className="mt-2 text-sm leading-relaxed text-slate-500">
          With{" "}
          <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono text-xs text-slate-300">
            RP_PROD_PASSWORD
          </code>{" "}
          set, anything that lands in the last ring — a promotion, a direct seed,
          even enabling auto-promote into it — must carry the production password.
        </p>
      </div>

      {/* leaving prod */}
      <div className="h-full rounded-xl border border-[#8b83ff]/20 bg-[#0a0c14] p-6">
        <svg viewBox="0 0 220 44" className="h-10 w-full" aria-hidden>
          <line x1="30" y1="26" x2="210" y2="26" stroke="#ffffff" strokeOpacity="0.12" strokeWidth="1.5" />
          <text x="10" y="12" fontSize="9" fill="#737373" fontFamily="var(--font-geist-mono), monospace">
            v2.13.9
          </text>
          <text x="210" y="12" fontSize="9" fill="#737373" textAnchor="end" fontFamily="var(--font-geist-mono), monospace">
            prod
          </text>
          <path
            className="pg-tick-out"
            d="M 40 24 l 4 4 l 8 -8"
            stroke={EM}
            strokeWidth="2"
            fill="none"
            strokeLinecap="round"
            strokeLinejoin="round"
            pathLength={1}
          />
          {/* the rollback: already home */}
          <circle className="pg-chip-out" cx="30" cy="26" r="5" fill="#f5f5f5" />
        </svg>
        <h3 className="mt-4 font-semibold text-slate-100">Leaving prod never waits</h3>
        <p className="mt-2 text-sm leading-relaxed text-slate-500">
          Rollbacks are deliberately exempt from the gate. When you are paged at
          3am, incident response is never blocked by a password prompt.
        </p>
      </div>
    </div>
  );
}
