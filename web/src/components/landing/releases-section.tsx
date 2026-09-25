"use client";

import { Reveal } from "./reveal";

const RELEASE_URL = "https://github.com/bwalia/ring-promoter/releases/tag/v1.0.2";
const RELEASES_INDEX = "https://github.com/bwalia/ring-promoter/releases";

const NOTES = [
  {
    title: "Orbit Deployer Board",
    body: "Earth sits at the centre as live users. Promotion rings are orbits — PROD closest in, then ACC, TEST, INT as outer spokes. Newest versions light solid; older versions stay hollow. Gates and the app roster ride alongside so you can see how far each release got.",
  },
  {
    title: "Lanes — multi-environment matrix",
    body: "NEW: a deployment matrix of APP × INT / TEST / ACC / PROD. When the same version spans rings, one version pill stretches across those columns. When prod lags, the break is obvious — no hunting through per-app pages.",
  },
  {
    title: "Descent & cinematic stage",
    body: "Descent treats promotion rings as orbits (with Lanes as the fallback). The Earth rings stage got more sky, quieter labels, and collision-aware leaders so the control room stays readable under load.",
  },
  {
    title: "Ops polish since v1.0.1",
    body: "In-place ring restart, external QA agent scaffolding, multi-arch images, longer helm waits for cold pulls, and clearer workstation secrets for production password gates.",
  },
] as const;

const SHOTS = [
  {
    src: "/releases/v1.0.2-orbit.png",
    alt: "Orbit Deployer Board — Earth at centre with PROD, ACC, TEST, and INT promotion rings",
    caption: "Orbit — Earth is live users; spokes show how far each app’s newest version climbed toward prod.",
    label: "Orbit",
  },
  {
    src: "/releases/v1.0.2-lanes.png",
    alt: "Lanes deployment matrix — apps across INT, TEST, ACC, and PROD with spanning version pills",
    caption: "Lanes — the same truth as a grid. Spanning pills mean parity; a split at PROD means it still has to earn the hop.",
    label: "Lanes",
  },
] as const;

/**
 * Happy Friday release companion — screenshots + notes that also work as
 * landing copy beside a walkthrough video.
 */
export function ReleasesSection() {
  return (
    <section
      id="releases"
      className="scroll-mt-20 border-b border-white/[0.07] bg-[#0b0b0c]"
    >
      <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-24">
        <Reveal variant="mask" className="mx-auto max-w-2xl text-center">
          <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-emerald-500/90">
            Happy Friday — Ring Promoter v1.0.2
          </p>
          <h2 className="mt-3 font-display text-balance text-3xl font-semibold tracking-tight text-neutral-50 sm:text-4xl">
            CI/CD Orbit and Lanes, side by side.
          </h2>
          <p className="mt-4 text-pretty text-[15px] leading-relaxed text-neutral-400 sm:text-base">
            Two views of the same promotion truth: where versions sit across
            int → test → acc → prod, and how far the newest build has earned.
            Use this as the companion to the walkthrough — pause on Orbit for
            the fleet, flip to Lanes when you need the matrix.
          </p>
          <div className="mt-6 flex flex-wrap items-center justify-center gap-3">
            <a
              href={RELEASE_URL}
              target="_blank"
              rel="noreferrer"
              className="rounded-md bg-neutral-100 px-3.5 py-2 text-[13px] font-medium text-neutral-900 transition-colors hover:bg-white"
            >
              Read v1.0.2 on GitHub
            </a>
            <a
              href={RELEASES_INDEX}
              target="_blank"
              rel="noreferrer"
              className="rounded-md border border-white/15 px-3.5 py-2 text-[13px] text-neutral-300 transition-colors hover:border-white/25 hover:text-neutral-100"
            >
              All releases ↗
            </a>
          </div>
          <p className="mt-3 font-mono text-[11px] text-neutral-600">
            Friday 25 Sep 2026
          </p>
        </Reveal>

        <div className="mt-12 grid grid-cols-1 gap-8 lg:grid-cols-2 lg:gap-10">
          {SHOTS.map((shot, i) => (
            <Reveal key={shot.src} delay={0.08 + i * 0.06}>
              <figure>
                <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[#090909] shadow-[0_0_0_1px_rgba(34,197,94,0.06),0_24px_80px_-36px_rgba(0,0,0,0.9)]">
                  <div className="flex items-center gap-2 border-b border-white/[0.06] px-3 py-2">
                    <span className="size-1.5 rounded-full bg-emerald-500/80" />
                    <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-neutral-500">
                      {shot.label}
                    </span>
                  </div>
                  {/* eslint-disable-next-line @next/next/no-img-element -- static export public asset */}
                  <img
                    src={shot.src}
                    alt={shot.alt}
                    width={1280}
                    height={800}
                    className="h-auto w-full"
                    loading="lazy"
                    decoding="async"
                  />
                </div>
                <figcaption className="mt-3 text-[13px] leading-relaxed text-neutral-500">
                  {shot.caption}
                </figcaption>
              </figure>
            </Reveal>
          ))}
        </div>

        <Reveal delay={0.12} className="mx-auto mt-14 max-w-3xl">
          <h3 className="font-display text-lg font-semibold tracking-tight text-neutral-100">
            What shipped
          </h3>
          <ul className="mt-6 space-y-5">
            {NOTES.map((note) => (
              <li key={note.title} className="border-l border-emerald-500/35 pl-4">
                <h4 className="text-sm font-semibold text-neutral-100">{note.title}</h4>
                <p className="mt-1.5 text-[13px] leading-relaxed text-neutral-500">
                  {note.body}
                </p>
              </li>
            ))}
          </ul>
        </Reveal>
      </div>
    </section>
  );
}
