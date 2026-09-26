import type { MetadataRoute } from "next";

// Makes the console installable as a PWA (Add to Home Screen / Install app).
// Paths honour NEXT_BASE_PATH so sub-path exports (GitHub Pages) still resolve.
const base = process.env.NEXT_BASE_PATH ?? "";

// Required for `output: "export"`: the manifest is emitted as a static file.
export const dynamic = "force-static";

export default function manifest(): MetadataRoute.Manifest {
  return {
    id: `${base}/`,
    name: "Ring Promoter",
    short_name: "Ring Promoter",
    description:
      "Promote application versions through int → test → acc → prod deployment rings.",
    start_url: `${base}/`,
    scope: `${base}/`,
    display: "standalone",
    background_color: "#0a0a0a",
    theme_color: "#0a0a0a",
    icons: [
      { src: `${base}/icons/icon-192.png`, sizes: "192x192", type: "image/png", purpose: "any" },
      { src: `${base}/icons/icon-512.png`, sizes: "512x512", type: "image/png", purpose: "any" },
      { src: `${base}/icons/icon-512.png`, sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
