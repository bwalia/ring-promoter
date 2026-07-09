"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

/**
 * Monospace version string (image tag, git ref or SHA) that copies itself on
 * click. Long values truncate; the full value is in the tooltip.
 */
export function VersionLabel({
  version,
  className,
}: {
  version: string;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);

  if (!version) {
    return <span className={cn("text-muted-foreground", className)}>—</span>;
  }

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(version);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard unavailable (non-secure context) — ignore.
    }
  };

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={copy}
          className={cn(
            "group/copy inline-flex min-w-0 max-w-full items-center gap-1 font-mono",
            "hover:text-foreground",
            className,
          )}
        >
          <span className="truncate">{version}</span>
          {copied ? (
            <Check aria-hidden className="size-3 shrink-0 text-status-good" />
          ) : (
            <Copy
              aria-hidden
              className="size-3 shrink-0 opacity-0 transition-opacity group-hover/copy:opacity-60"
            />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-96 break-all font-mono">
        {version} {copied ? "· copied!" : "· click to copy"}
      </TooltipContent>
    </Tooltip>
  );
}
