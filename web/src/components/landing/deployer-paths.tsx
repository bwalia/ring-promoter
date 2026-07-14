"use client";

import { useRef } from "react";
import { Container, Workflow } from "lucide-react";
import { gsap, useGSAP, MOTION_OK } from "./gsap/setup";

// "Different machinery, identical contract": one control plane fans out over
// drawn paths to two very different deploy mechanisms — and both emit the
// SAME normalized result row at the same instant. Hovering a card sends a
// dispatch pulse down its path (cursor motion that means something).

const CARDS = [
  {
    icon: Container,
    title: "Kubernetes apps",
    body: (
      <>
        The kubectl deployer runs{" "}
        <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono text-xs text-slate-300">
          set image
        </code>{" "}
        +{" "}
        <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono text-xs text-slate-300">
          rollout status
        </code>
        , authenticated in-cluster via the pod&rsquo;s ServiceAccount.
        Battle-tested rollout semantics with a tiny dependency tree.
      </>
    ),
  },
  {
    icon: Workflow,
    title: "VM apps with existing CI",
    body: (
      <>
        The GitHub Actions deployer dispatches the workflow you already have and
        waits for the run to conclude — same health checks, same auto-rollback.
        Seeded versions are validated against the repo&rsquo;s real branches and
        tags, so a typo&rsquo;d ref never launches a doomed run.
      </>
    ),
  },
] as const;

const PATHS = ["M 50 2 C 50 22, 25 16, 25 38", "M 50 2 C 50 22, 75 16, 75 38"];

export function DeployerPaths() {
  const scope = useRef<HTMLDivElement>(null);

  const { contextSafe } = useGSAP(
    () => {
      const q = gsap.utils.selector(scope);
      const mm = gsap.matchMedia();
      mm.add(MOTION_OK, () => {
        gsap.set(q(".dp-node"), { opacity: 0, y: 6 });
        gsap.set(q(".dp-path"), { strokeDasharray: 1, strokeDashoffset: 1 });
        gsap.set(q(".dp-card"), { opacity: 0.45 });
        gsap.set(q(".dp-row"), { opacity: 0, scale: 0.95, transformOrigin: "left center" });

        const tl = gsap.timeline({
          defaults: { ease: "commit" },
          scrollTrigger: { trigger: scope.current, start: "top 75%", once: true },
        });
        tl.to(q(".dp-node"), { opacity: 1, y: 0, duration: 0.35 }, 0);
        tl.to(q(".dp-path"), { strokeDashoffset: 0, duration: 0.5, ease: "none", stagger: 0.12 }, 0.25);
        tl.to(q(".dp-card"), { opacity: 1, duration: 0.4, stagger: 0.12 }, 0.55);
        // deliberately NO stagger: the identical contract lands at one instant
        tl.to(q(".dp-row"), { opacity: 1, scale: 1, duration: 0.3 }, 1.1);
      });
    },
    { scope },
  );

  const pulse = contextSafe((i: number) => {
    if (!window.matchMedia(MOTION_OK).matches) return;
    gsap.fromTo(
      `.dp-pulse-${i}`,
      { strokeDashoffset: 8, opacity: 1 },
      {
        strokeDashoffset: -100,
        duration: 0.7,
        ease: "none",
        overwrite: "auto",
        onComplete: () => gsap.set(`.dp-pulse-${i}`, { opacity: 0 }),
      },
    );
  });

  return (
    <div ref={scope} className="mt-12">
      <div className="dp-node mx-auto flex w-fit items-center gap-2 rounded-full border border-white/15 bg-[#0f131e] px-3 py-1.5 font-mono text-xs text-slate-300">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden className="size-3.5">
          <circle cx="12" cy="12" r="10" stroke="#22c55e" strokeWidth="1.5" strokeDasharray="2 3.2" />
          <circle cx="12" cy="12" r="5.5" stroke="#e5e5e5" strokeWidth="1.5" />
        </svg>
        ring-promoter
      </div>
      <svg
        viewBox="0 0 100 40"
        preserveAspectRatio="none"
        aria-hidden
        className="-my-0.5 block h-14 w-full"
      >
        {PATHS.map((d, i) => (
          <path
            key={i}
            className="dp-path"
            d={d}
            fill="none"
            stroke="#ffffff"
            strokeOpacity="0.15"
            strokeWidth="1.2"
            vectorEffect="non-scaling-stroke"
            pathLength={1}
          />
        ))}
        {PATHS.map((d, i) => (
          <path
            key={`p${i}`}
            className={`dp-pulse-${i}`}
            d={d}
            fill="none"
            stroke="#22c55e"
            strokeWidth="1.6"
            vectorEffect="non-scaling-stroke"
            pathLength={100}
            strokeDasharray="8 92"
            strokeDashoffset={8}
            opacity={0}
          />
        ))}
      </svg>
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        {CARDS.map(({ icon: Icon, title, body }, i) => (
          <div
            key={title}
            className="dp-card h-full rounded-xl border border-white/[0.07] bg-[#0f131e] p-6 transition-colors hover:border-white/[0.14]"
            onMouseEnter={() => pulse(i)}
          >
            <div className="flex items-center gap-3">
              <span className="flex size-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04]">
                <Icon aria-hidden className="size-4 text-slate-300" />
              </span>
              <h3 className="font-semibold text-slate-100">{title}</h3>
            </div>
            <p className="mt-4 text-sm leading-relaxed text-slate-500">{body}</p>
            <p className="dp-row mt-4 border-t border-white/[0.07] pt-3 font-mono text-[11px] text-[#a9a3ff]">
              ✓ deployed v2.14.0 · healthy — result recorded
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}
