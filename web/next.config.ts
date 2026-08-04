import path from "node:path";
import type { NextConfig } from "next";

// Two modes:
//  - `npm run dev`: a normal Next dev server; /api and /healthz are proxied to
//    the Go server on :8080 (start it with `go run ./cmd/ringpromoter`).
//  - `npm run build:embed` (NEXT_OUTPUT=export): a fully static export that is
//    copied into internal/web/static and embedded into the Go binary, so the
//    UI and API share one origin in production.
const isExport = process.env.NEXT_OUTPUT === "export";

// Optional sub-path prefix for exports hosted somewhere other than the domain
// root (e.g. NEXT_BASE_PATH=/ring-promoter for GitHub Pages). The embedded UI
// build (scripts/embed.sh) leaves this unset and is unaffected.
const basePath = process.env.NEXT_BASE_PATH ?? "";

const nextConfig: NextConfig = {
  turbopack: { root: path.join(__dirname) },
  ...(basePath ? { basePath } : {}),
  ...(isExport
    ? { output: "export" as const }
    : {
        async rewrites() {
          const backend = process.env.RP_BACKEND ?? "http://localhost:8080";
          return [
            { source: "/api/:path*", destination: `${backend}/api/:path*` },
            { source: "/healthz", destination: `${backend}/healthz` },
            { source: "/version", destination: `${backend}/version` },
          ];
        },
      }),
};

export default nextConfig;
