"use client";

import { useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { useJobStream } from "@/lib/job-stream";

// Holds the app-wide SSE job stream open while signed in; the hook needs the
// QueryClient, so it lives in a child of the provider.
function JobStream() {
  useJobStream();
  return null;
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: 1,
            staleTime: 5_000,
            refetchOnWindowFocus: true,
          },
        },
      }),
  );

  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
    >
      <QueryClientProvider client={queryClient}>
        <JobStream />
        <TooltipProvider delayDuration={200}>{children}</TooltipProvider>
        <Toaster richColors closeButton position="bottom-right" />
      </QueryClientProvider>
    </ThemeProvider>
  );
}
