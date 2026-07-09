"use client";

import { AlertTriangle, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";

export function ErrorState({
  title = "Something went wrong",
  message,
  onRetry,
}: {
  title?: string;
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-8 text-center">
      <AlertTriangle aria-hidden className="size-8 text-status-serious" />
      <div>
        <p className="font-medium">{title}</p>
        {message && (
          <p className="mt-1 max-w-md text-sm text-muted-foreground">
            {message}
          </p>
        )}
      </div>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RotateCw aria-hidden className="size-4" /> Retry
        </Button>
      )}
    </div>
  );
}
