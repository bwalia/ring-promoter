import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// Build output goes INTO the Go module so `go:embed dist` picks it up at
// compile time (embed cannot reach outside internal/web/). The dist is
// committed so a plain `go build` always works; the Docker node stage
// rebuilds it from source at image-build time (source of truth).
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
      "/version": "http://localhost:8080",
    },
  },
});
