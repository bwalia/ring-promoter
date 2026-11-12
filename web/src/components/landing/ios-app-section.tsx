import Link from "next/link";
import { Reveal } from "@/components/landing/reveal";
import { AppStoreBadge } from "@/components/app-store-badge";
import { cn } from "@/lib/utils";

const shots = [
  {
    src: "/ios/iphone-overview.png",
    alt: "Ring Promoter on iPhone: overview of applications and the version live in each ring",
    width: 506,
    height: 1100,
    label: "iPhone",
  },
  {
    src: "/ios/ipad-rings-universe.png",
    alt: "Ring Promoter on iPad: the rings universe view showing versions moving through int, test, acc and prod",
    width: 1050,
    height: 1400,
    label: "iPad",
  },
  {
    src: "/ios/iphone-production-guard.png",
    alt: "Ring Promoter on iPhone: the production guard asking for confirmation before promoting to prod",
    width: 506,
    height: 1100,
    label: "iPhone",
  },
] as const;

export function IosAppSection() {
  return (
    <section id="ios" className="scroll-mt-20 border-y border-white/[0.07] bg-[#0b0b0c]">
      <div className="mx-auto grid max-w-6xl grid-cols-1 items-center gap-12 px-4 py-24 sm:px-6 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        <Reveal variant="mask">
          <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-emerald-500/90">
            iPhone &amp; iPad
          </p>
          <h2 className="mt-3 text-balance font-display text-[length:var(--rp-font-h2)] font-bold tracking-[-0.02em] text-neutral-50">
            Your release pipeline, in your pocket.
          </h2>
          <p className="mt-4 text-pretty text-[length:var(--rp-font-lead)] leading-[1.7] text-neutral-400">
            See every ring, promote one hop at a time with the same gates enforced,
            and watch the job run live — or roll back from wherever you are. No
            server handy? Explore everything in offline demo mode.
          </p>
          <ul className="mt-6 space-y-2 text-[13px] text-neutral-500">
            <li>• Connects to your own control plane with its API token</li>
            <li>• Production promotions still require the prod password</li>
            <li>• Free, no account, no tracking — iOS 17 or later</li>
          </ul>
          <div className="mt-8 flex flex-wrap items-center gap-4">
            <AppStoreBadge />
            <Link
              href="/support"
              className="text-[13px] text-neutral-400 transition-colors hover:text-neutral-100"
            >
              App support →
            </Link>
          </div>
        </Reveal>
        <Reveal delay={0.1}>
          <div className="flex items-end justify-center gap-3 sm:gap-5">
            {shots.map((shot, i) => (
              <div
                key={shot.src}
                className={cn(
                  "overflow-hidden rounded-[1.4rem] border border-white/10 bg-black shadow-2xl shadow-black/60",
                  shot.label === "iPad" ? "w-[46%]" : "w-[24%]",
                  i === 0 && "translate-y-4",
                  i === 2 && "translate-y-8",
                )}
              >
                {/* eslint-disable-next-line @next/next/no-img-element -- static export public asset */}
                <img
                  src={shot.src}
                  alt={shot.alt}
                  width={shot.width}
                  height={shot.height}
                  className="h-auto w-full"
                  loading="lazy"
                  decoding="async"
                />
              </div>
            ))}
          </div>
        </Reveal>
      </div>
    </section>
  );
}
