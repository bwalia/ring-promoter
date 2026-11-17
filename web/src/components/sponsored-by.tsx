import { cn } from "@/lib/utils";

export const WORKSTATION_URL = "https://www.workstation.co.uk/en";

/**
 * Sponsor credit used in every public and console footer.
 */
export function SponsoredBy({
  className,
  linkClassName,
}: {
  className?: string;
  linkClassName?: string;
}) {
  return (
    <p className={cn("text-xs", className)}>
      Sponsored by{" "}
      <a
        href={WORKSTATION_URL}
        target="_blank"
        rel="noreferrer"
        className={cn(
          "font-medium underline-offset-2 transition-colors hover:underline",
          linkClassName,
        )}
      >
        Workstation
      </a>
    </p>
  );
}
