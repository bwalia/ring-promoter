"use client";

import { Reveal } from "./reveal";

const VIDEO_ID = "IzhvmrwEebE";

export function WatchVideo() {
  return (
    <section id="watch" className="scroll-mt-20 border-b border-white/[0.07]">
      <div className="mx-auto max-w-4xl px-4 py-12 sm:px-6 sm:py-16">
        <Reveal variant="mask" className="mx-auto max-w-2xl text-center">
          <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-emerald-500/90">
            Watch
          </p>
          <h2 className="mt-3 font-display text-balance text-2xl font-semibold tracking-tight text-neutral-50 sm:text-3xl">
            See Ring Promoter in action.
          </h2>
          <p className="mt-3 text-pretty text-[15px] leading-relaxed text-neutral-400">
            A short walkthrough of the promotion protocol, live jobs, and the
            Rings of Applications console.
          </p>
        </Reveal>
        <Reveal delay={0.1} className="mt-8">
          <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[#0b0b0c] shadow-[0_0_0_1px_rgba(34,197,94,0.08),0_24px_80px_-32px_rgba(0,0,0,0.85)]">
            <div className="relative aspect-video">
              <iframe
                src={`https://www.youtube.com/embed/${VIDEO_ID}`}
                title="Ring Promoter — product overview"
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
                allowFullScreen
                loading="lazy"
                className="absolute inset-0 size-full"
              />
            </div>
          </div>
        </Reveal>
      </div>
    </section>
  );
}
