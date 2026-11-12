import { cn } from "@/lib/utils";

/** Public App Store listing for the iPhone and iPad app. */
export const APP_STORE_URL =
  "https://apps.apple.com/us/app/ring-promoter/id6798293431";

function AppleLogo({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 384 512"
      aria-hidden
      className={className}
      fill="currentColor"
    >
      <path d="M318.7 268.7c-.2-36.7 16.4-64.4 50-84.8-18.8-26.9-47.2-41.7-84.7-44.6-35.5-2.8-74.3 20.7-88.5 20.7-15 0-49.4-19.7-76.4-19.7C63.3 141.2 4 184.8 4 273.5q0 39.3 14.4 81.2c12.8 36.7 59 126.7 107.2 125.2 25.2-.6 43-17.9 75.8-17.9 31.8 0 48.3 17.9 76.4 17.9 48.6-.7 90.4-82.5 102.6-119.3-65.2-30.7-61.7-90-61.7-91.9zm-56.6-164.2c27.3-32.4 24.8-61.9 24-72.5-24.1 1.4-52 16.4-67.9 34.9-17.5 19.8-27.8 44.3-25.6 71.9 26.1 2 49.9-11.4 69.5-34.3z" />
    </svg>
  );
}

/**
 * "Download on the App Store" badge, drawn in the style of Apple's black badge.
 * Used in the header and footer of every published page.
 *
 * `compact` drops the "Download on the" line below the `sm` breakpoint so the
 * badge still fits in a phone-width header next to the brand.
 */
export function AppStoreBadge({
  className,
  size = "md",
  compact = false,
}: {
  className?: string;
  size?: "sm" | "md";
  compact?: boolean;
}) {
  return (
    <a
      href={APP_STORE_URL}
      target="_blank"
      rel="noreferrer"
      aria-label="Download Ring Promoter on the App Store for iPhone and iPad"
      className={cn(
        "inline-flex shrink-0 items-center gap-2 whitespace-nowrap rounded-lg border border-white/25 bg-black text-white transition-colors hover:border-white/50 hover:bg-neutral-950",
        size === "sm" ? "h-9 px-2.5" : "h-11 px-3.5",
        compact && "h-8 gap-1.5 px-2 sm:h-9 sm:gap-2 sm:px-2.5",
        className,
      )}
    >
      <AppleLogo
        className={cn(
          size === "sm" ? "size-4" : "size-5",
          compact && "size-3.5 sm:size-4",
        )}
      />
      <span className="flex flex-col items-start leading-none">
        <span
          className={cn(
            "font-medium",
            size === "sm" ? "text-[8px]" : "text-[10px]",
            compact && "hidden sm:block",
          )}
        >
          Download on the
        </span>
        <span
          className={cn(
            "font-semibold tracking-tight",
            size === "sm" ? "text-[14px]" : "text-[18px]",
            compact && "text-[12px] sm:text-[14px]",
          )}
        >
          App Store
        </span>
      </span>
    </a>
  );
}
