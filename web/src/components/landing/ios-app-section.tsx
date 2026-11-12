import Link from "next/link";
import { Reveal } from "@/components/landing/reveal";
import { cn } from "@/lib/utils";

/** Public App Store listing for the iPhone and iPad app. */
export const APP_STORE_URL = "https://apps.apple.com/us/app/ring-promoter/id6798293431";

function AppleLogo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 384 512" aria-hidden className={className} fill="currentColor">
      <path d="M318.7 268.7c-.2-36.7 16.4-64.4 50-84.8-18.8-26.9-47.2-41.7-84.7-44.6-35.5-2.8-74.3 20.7-88.5 20.7-15 0-49.4-19.7-76.4-19.7C63.3 141.2 4 184.8 4 273.5q0 39.3 14.4 81.2c12.8 36.7 59 126.7 107.2 125.2 25.2-.6 43-17.9 75.8-17.9 31.8 0 48.3 17.9 76.4 17.9 48.6-.7 90.4-82.5 102.6-119.3-65.2-30.7-61.7-90-61.7-91.9zm-56.6-164.2c27.3-32.4 24.8-61.9 24-72.5-24.1 1.4-52 16.4-67.9 34.9-17.5 19.8-27.8 44.3-25.6 71.9 26.1 2 49.9-11.4 69.5-34.3z" />
    </svg>
  );
}

/** "Download on the App Store" badge, drawn in the style of Apple's black badge. */
export function AppStoreBadge({
  className,
  size = "md",
}: {
  className?: string;
  size?: "sm" | "md";
}) {
  return (
    <a
      href={APP_STORE_URL}
      target="_blank"
      rel="noreferrer"
      aria-label="Download Ring Promoter on the App Store for iPhone and iPad"
      className={cn(
        "inline-flex items-center gap-2 rounded-lg border border-white/25 bg-black text-white transition-colors hover:border-white/50 hover:bg-neutral-950",
        size === "sm" ? "h-9 px-2.5" : "h-11 px-3.5",
        className,
      )}
    >
      <AppleLogo className={size === "sm" ? "size-4" : "size-5"} />
      <span className="flex flex-col items-start leading-none">
        <span className={cn("font-medium", size === "sm" ? "text-[8px]" : "text-[10px]")}>
          Download on the
        </span>
        <span
          className={cn(
            "font-semibold tracking-tight",
            size === "sm" ? "text-[14px]" : "text-[18px]",
          )}
        >
          App Store
        </span>
      </span>
    </a>
  );
}

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
