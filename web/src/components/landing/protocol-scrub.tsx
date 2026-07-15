"use client";

import { useRef } from "react";
import { gsap, useGSAP } from "./gsap/setup";
import { Reveal } from "./reveal";
import { cn } from "@/lib/utils";

// The flagship scroll moment: the section pins (lg+, motion-ok only) and a
// scrubbed timeline walks through the five promotion rules while one shared
// mini-rail acts each rule out — hop, prove, earn, roll back, record.
//
// The SVG's static markup is the COMPLETED story (chip at acc, checks drawn,
// ledger written) so no-JS readers and reduced-motion users get a finished
// diagram; the motion branch resets it to zero and lets scroll earn it back.

const RULES = [
  ["One ring at a time", "Promote always targets the next ring in the pipeline. Skipping is impossible by design — not by policy."],
  ["The source must prove it", "A live health check on the source ring gates every promotion before anything deploys."],
  ["The target must earn it", "After deploying, health checks run with configurable retries before the hop is called good."],
  ["Failure undoes itself", "If the target stays unhealthy, it is rolled back to its previous version automatically."],
  ["Everything is written down", "Every seed, promote and rollback lands in history — success or failure, per app, per ring."],
] as const;

const LEDGER = [
  ["14:02:11", "✓ promote int → test · healthy", "text-[#a9a3ff]"],
  ["14:02:19", "✓ promote test → acc · checks 3/3", "text-[#a9a3ff]"],
  ["14:02:27", "✗ promote acc → prod · rolled back", "text-red-400"],
  ["14:02:27", "history updated · rolled_back=true", "text-slate-400"],
] as const;

const X = [20, 140, 260, 380]; // station x-positions in the 400-unit viewBox
const RING_KEYS = ["int", "test", "acc", "prod"] as const;

const EM = "#22c55e";
const RD = "#ef4444";
const DOT_Z = "#52525b";
const RING_Z = "#3f3f46";

export function ProtocolSection() {
  const scope = useRef<HTMLElement>(null);
  const pinRef = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const q = gsap.utils.selector(scope);
      const mm = gsap.matchMedia();

      mm.add("(min-width: 1024px) and (prefers-reduced-motion: no-preference)", () => {
        // ── reset the completed story to its zero pose ──
        gsap.set(q(".pr-rule"), { opacity: 0.3, borderColor: "rgba(255,255,255,0.08)" });
        gsap.set(q(".pr-seg"), { strokeDasharray: 1, strokeDashoffset: 1 });
        gsap.set(q(".pr-st"), { scale: 0, transformOrigin: "50% 50%" });
        gsap.set(q(".pr-lbl"), { opacity: 0 });
        gsap.set(q(".pr-chip"), { x: 0, opacity: 0 });
        gsap.set(q(".pr-transient"), { opacity: 0 });
        gsap.set(q(".pr-tick-src, .pr-check"), { opacity: 0 });
        gsap.set(q(".pr-dot-1, .pr-dot-2"), { fill: DOT_Z });
        gsap.set(q(".pr-ring-1, .pr-ring-2"), { stroke: RING_Z });
        gsap.set(q(".pr-led"), { opacity: 0, y: 8 });

        const tl = gsap.timeline({
          defaults: { ease: "commit", duration: 0.3 },
          scrollTrigger: {
            trigger: pinRef.current,
            pin: true,
            start: "top 56px", // sits flush under the sticky nav
            end: "+=2800",
            scrub: 0.7,
            anticipatePin: 1,
          },
        });

        const activate = (i: number, pos: number) => {
          tl.to(q(".pr-rule"), { opacity: 0.3, borderColor: "rgba(255,255,255,0.08)", duration: 0.25 }, pos);
          tl.to(q(`.pr-rule-${i}`), { opacity: 1, borderColor: "rgba(52,211,153,0.45)", duration: 0.25 }, "<");
        };

        // ── intro: the instrument assembles (echoes the hero grammar) ──
        tl.to(q(".pr-seg"), { strokeDashoffset: 0, duration: 0.4, stagger: 0.1, ease: "none" }, 0);
        tl.to(q(".pr-st"), { scale: 1, duration: 0.25, stagger: 0.1 }, 0.05);
        tl.to(q(".pr-lbl"), { opacity: 1, duration: 0.2, stagger: 0.1 }, 0.15);
        tl.fromTo(q(".pr-chip"), { x: -24 }, { x: 0, opacity: 1, duration: 0.35 }, 0.55);

        // ── 01: one ring at a time — a hop lands; a skip is refused ──
        activate(0, 0.95);
        tl.to(q(".pr-chip"), { x: 120, duration: 0.5, ease: "advance" }, 1.15);
        tl.to(q(".pr-dot-1"), { fill: EM, duration: 0.15 }, 1.6);
        tl.to(q(".pr-ring-1"), { stroke: EM, duration: 0.15 }, "<");
        tl.to(q(".pr-skip"), { opacity: 1, duration: 0.25 }, 1.85);
        tl.fromTo(
          q(".pr-skip-x"),
          { scale: 0.4, transformOrigin: "50% 50%" },
          { opacity: 1, scale: 1, duration: 0.18 },
          2.1,
        );
        tl.to(q(".pr-skip, .pr-skip-x"), { opacity: 0, duration: 0.25 }, 2.5);

        // ── 02: the source must prove it — pulse, tick, path opens ──
        activate(1, 2.65);
        tl.fromTo(
          q(".pr-pulse"),
          { opacity: 0.9, scale: 0.5, transformOrigin: "50% 50%" },
          // immediateRender would paint the "from" pose at build time,
          // leaking a green ring before this chapter is reached
          { opacity: 0, scale: 1.9, duration: 0.5, ease: "none", immediateRender: false },
          2.85,
        );
        tl.to(q(".pr-tick-src"), { opacity: 1, duration: 0.05 }, 3.15);
        tl.fromTo(
          q(".pr-tick-src"),
          { strokeDasharray: 1, strokeDashoffset: 1 },
          { strokeDashoffset: 0, duration: 0.25, ease: "none" },
          "<",
        );
        tl.to(q(".pr-seg-1"), { stroke: EM, strokeOpacity: 0.5, duration: 0.2 }, 3.45);

        // ── 03: the target must earn it — three checks, then commit ──
        activate(2, 3.7);
        tl.to(q(".pr-chip"), { x: 240, duration: 0.5, ease: "advance" }, 3.9);
        for (let k = 0; k < 3; k++) {
          tl.to(q(`.pr-check-${k}`), { opacity: 1, duration: 0.05 }, 4.45 + k * 0.2);
          tl.fromTo(
            q(`.pr-check-${k}`),
            { strokeDasharray: 1, strokeDashoffset: 1 },
            { strokeDashoffset: 0, duration: 0.15, ease: "none" },
            "<",
          );
        }
        tl.to(q(".pr-dot-2"), { fill: EM, duration: 0.15 }, 5.1);
        tl.to(q(".pr-ring-2"), { stroke: EM, duration: 0.15 }, "<");

        // ── 04: failure undoes itself — red at prod, decisive snap back ──
        activate(3, 5.25);
        tl.to(q(".pr-chip"), { x: 360, duration: 0.45, ease: "advance" }, 5.45);
        tl.to(q(".pr-ring-3"), { stroke: RD, duration: 0.12 }, 5.9);
        tl.to(q(".pr-dot-3"), { fill: RD, duration: 0.12 }, "<");
        tl.fromTo(
          q(".pr-fail-x"),
          { scale: 0.4, transformOrigin: "50% 50%" },
          { opacity: 1, scale: 1, duration: 0.15 },
          6.05,
        );
        tl.to(q(".pr-chip"), { x: 240, duration: 0.3, ease: "contain" }, 6.3);
        tl.to(q(".pr-dot-3"), { fill: DOT_Z, duration: 0.2 }, 6.6);
        tl.to(q(".pr-ring-3"), { stroke: RING_Z, duration: 0.2 }, "<");
        tl.to(q(".pr-fail-x"), { opacity: 0, duration: 0.2 }, 6.65);

        // ── 05: everything is written down — the ledger fills, then stillness ──
        activate(4, 6.8);
        tl.to(q(".pr-led"), { opacity: 1, y: 0, duration: 0.3, stagger: 0.2 }, 7.0);
        tl.to({}, { duration: 0.5 }, 7.9); // hold the finished record before unpinning
      });
    },
    { scope },
  );

  return (
    <section id="protocol" ref={scope} className="scroll-mt-20">
      {/* ── <lg / fallback: the stacked rule cards ── */}
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6 lg:hidden">
        <ProtocolHead />
        <div className="mt-12 grid grid-cols-1 gap-px overflow-hidden rounded-xl border border-white/[0.07] bg-white/[0.07] sm:grid-cols-2">
          {RULES.map(([title, body], i) => (
            <Reveal key={title} delay={i * 0.05} className="bg-[#0f131e] p-5">
              <p className="font-mono text-xs text-[#8b83ff]/80">{String(i + 1).padStart(2, "0")}</p>
              <h3 className="mt-3 text-sm font-semibold text-slate-100">{title}</h3>
              <p className="mt-2 text-[13px] leading-relaxed text-slate-500">{body}</p>
            </Reveal>
          ))}
        </div>
      </div>

      {/* ── lg+: the pinned stage ── */}
      <div className="hidden lg:block">
        <div
          ref={pinRef}
          className="mx-auto flex h-[calc(100vh-56px)] max-w-6xl flex-col justify-center px-6"
        >
          <ProtocolHead />
          <div className="mt-10 grid grid-cols-[1fr_1.15fr] items-center gap-14">
            <ol className="space-y-4">
              {RULES.map(([title, body], i) => (
                <li key={title} className={`pr-rule pr-rule-${i} border-l border-white/[0.08] py-1 pl-5`}>
                  <div className="flex items-baseline gap-3">
                    <span className="font-mono text-xs text-[#8b83ff]/80">
                      {String(i + 1).padStart(2, "0")}
                    </span>
                    <h3 className="text-[15px] font-semibold text-slate-100">{title}</h3>
                  </div>
                  <p className="mt-1 pl-8 text-[13px] leading-relaxed text-slate-500">{body}</p>
                </li>
              ))}
            </ol>

            <div className="rounded-2xl border border-white/[0.07] bg-[#0f131e] p-6">
              <p className="font-mono text-[10px] uppercase tracking-widest text-slate-600">
                protocol demo · payments-api
              </p>
              <StageSvg />
              <div className="mt-2 space-y-1.5 border-t border-white/[0.07] pt-3 font-mono text-[11px]">
                {LEDGER.map(([time, text, tone]) => (
                  <p key={text} className="pr-led flex gap-3 whitespace-nowrap">
                    <span className="text-slate-600">{time}</span>
                    <span className={cn("truncate", tone)}>{text}</span>
                  </p>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function ProtocolHead() {
  return (
    <div className="max-w-2xl">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-[#8b83ff]/90">
        The protocol
      </p>
      <h2 className="mt-3 text-balance text-3xl font-semibold tracking-tight text-slate-50 sm:text-4xl">
        Promotion is a protocol, not a script.
      </h2>
      <p className="mt-4 text-pretty leading-relaxed text-slate-400">
        Five rules, enforced by the control plane on every operation. They are
        what make a promotion safe to run at 5pm on a Friday.
      </p>
    </div>
  );
}

/** Static pose = the finished story; the motion branch rewinds it. */
function StageSvg() {
  return (
    <svg viewBox="0 0 400 118" className="mt-3 w-full" aria-hidden>
      {/* rail segments */}
      {[0, 1, 2].map((i) => (
        <line
          key={i}
          className={`pr-seg pr-seg-${i}`}
          x1={X[i] + 14}
          y1="70"
          x2={X[i + 1] - 14}
          y2="70"
          stroke="#ffffff"
          strokeOpacity="0.14"
          strokeWidth="1.5"
          pathLength={1}
        />
      ))}

      {/* transient annotations (hidden in the static pose) */}
      <path
        className="pr-skip pr-transient"
        d="M 140 54 Q 260 6 380 54"
        fill="none"
        stroke="#f59e0b"
        strokeWidth="1.5"
        strokeDasharray="4 5"
        opacity="0"
      />
      <g className="pr-skip-x pr-transient" stroke={RD} strokeWidth="2" strokeLinecap="round" opacity="0">
        <line x1="254" y1="18" x2="266" y2="30" />
        <line x1="266" y1="18" x2="254" y2="30" />
      </g>
      <circle
        className="pr-pulse pr-transient"
        cx="140"
        cy="70"
        r="10"
        fill="none"
        stroke={EM}
        strokeWidth="1.5"
        opacity="0"
      />
      <g className="pr-fail-x pr-transient" stroke={RD} strokeWidth="2" strokeLinecap="round" opacity="0">
        <line x1="374" y1="26" x2="386" y2="38" />
        <line x1="386" y1="26" x2="374" y2="38" />
      </g>

      {/* proof marks (visible in the static pose) */}
      <path
        className="pr-tick-src"
        d="M 132 36 l 5 5 l 10 -10"
        fill="none"
        stroke={EM}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        pathLength={1}
      />
      {[0, 1, 2].map((k) => (
        <path
          key={k}
          className={`pr-check pr-check-${k}`}
          d={`M ${234 + k * 20} 36 l 4 4 l 7 -7`}
          fill="none"
          stroke={EM}
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          pathLength={1}
        />
      ))}

      {/* stations */}
      {X.map((x, i) => (
        <g key={i} className="pr-st">
          <circle
            className={`pr-ring-${i}`}
            cx={x}
            cy="70"
            r="10"
            fill="none"
            stroke={i < 3 ? EM : RING_Z}
            strokeWidth="1.5"
          />
          <circle className={`pr-dot-${i}`} cx={x} cy="70" r="4" fill={i < 3 ? EM : DOT_Z} />
        </g>
      ))}
      {X.map((x, i) => (
        <text
          key={i}
          className="pr-lbl"
          x={x}
          y="100"
          textAnchor="middle"
          fontSize="10"
          fill="#a1a1aa"
          fontFamily="var(--font-geist-mono), monospace"
        >
          {RING_KEYS[i]}
        </text>
      ))}

      {/* the version chip (static pose: contained at acc after the rollback) */}
      <g className="pr-chip" transform="translate(240, 0)">
        <text
          x="20"
          y="52"
          textAnchor="middle"
          fontSize="9"
          fill="#e5e5e5"
          fontFamily="var(--font-geist-mono), monospace"
        >
          v2.14.0
        </text>
        <circle cx="20" cy="70" r="6.5" fill="#f5f5f5" />
        <circle cx="20" cy="70" r="2.2" fill="#0f131e" />
      </g>
    </svg>
  );
}
