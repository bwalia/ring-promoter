"use client";

import { useRef } from "react";
import Link from "next/link";
import { gsap, useGSAP, MOTION_OK } from "./gsap/setup";

// "The operator takes control": the command resolves in one fast pass (no
// slow typing), the ready caret lands, and only then do the actions become
// available. Static markup is the ready terminal.

export function CtaTerminal({ github }: { github: string }) {
  const scope = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const q = gsap.utils.selector(scope);
      const mm = gsap.matchMedia();
      mm.add(MOTION_OK, () => {
        gsap.set(q(".ct-cmd"), { clipPath: "inset(0 100% 0 0)" });
        gsap.set(q(".ct-out"), { opacity: 0 });
        gsap.set(q(".ct-caret"), { opacity: 0 });
        gsap.set(q(".ct-btns"), { opacity: 0, y: 10 });

        const tl = gsap.timeline({
          scrollTrigger: { trigger: scope.current, start: "top 78%", once: true },
        });
        tl.to(q(".ct-cmd"), { clipPath: "inset(0 -2% 0 0)", duration: 0.45, ease: "none" }, 0.2);
        tl.to(q(".ct-out"), { opacity: 1, duration: 0.3, ease: "commit" }, 0.75);
        tl.to(q(".ct-caret"), { opacity: 1, duration: 0.15 }, 0.95);
        tl.to(q(".ct-btns"), { opacity: 1, y: 0, duration: 0.45, ease: "commit" }, 1.05);
      });
    },
    { scope },
  );

  return (
    <div ref={scope}>
      <div className="mx-auto mt-8 max-w-xl overflow-hidden rounded-xl border border-white/10 bg-[#0b0b0c] text-left">
        <pre className="overflow-x-auto px-4 py-3.5 font-mono text-xs leading-relaxed">
          <code>
            <span className="text-neutral-600">$ </span>
            <span className="ct-cmd inline-block text-neutral-100">
              go run ./cmd/ringpromoter --config config.yaml
            </span>
            <span className="ct-caret text-emerald-400"> ▊</span>
            {"\n"}
            <span className="ct-out text-neutral-600">
              # → http://localhost:8080 · token: local-dev-token
            </span>
          </code>
        </pre>
      </div>
      <div className="ct-btns mt-8 flex flex-wrap items-center justify-center gap-3">
        <Link
          href="/"
          className="rounded-md bg-neutral-100 px-5 py-2.5 text-sm font-medium text-neutral-900 transition-colors hover:bg-white"
        >
          Open the console
        </Link>
        <a
          href={github}
          target="_blank"
          rel="noreferrer"
          className="rounded-md border border-white/15 px-5 py-2.5 text-sm font-medium text-neutral-200 transition-colors hover:bg-white/[0.06]"
        >
          View on GitHub
        </a>
      </div>
    </div>
  );
}
