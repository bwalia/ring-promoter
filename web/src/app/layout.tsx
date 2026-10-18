import type { Metadata, Viewport } from "next";
import { JetBrains_Mono, Poppins, Unbounded } from "next/font/google";
import Script from "next/script";
import "./globals.css";
import { Providers } from "@/components/providers";
import { ServiceWorkerRegister } from "@/components/service-worker-register";

// Match workstation-website: Poppins body, Unbounded display, JetBrains Mono.
const poppins = Poppins({
  variable: "--font-body",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  display: "swap",
});

const unbounded = Unbounded({
  variable: "--font-display",
  subsets: ["latin"],
  weight: ["500", "700"],
  display: "swap",
});

// Data face. Taller x-height and unambiguous 0/O and 1/l — numerals are
// read at 11px in latency chips. Variable name kept for SVG fontFamily refs.
const jetbrainsMono = JetBrains_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  display: "swap",
});

export const metadata: Metadata = {
  title: "Ring Promoter",
  description:
    "Promote application versions through int → test → acc → prod deployment rings.",
  applicationName: "Ring Promoter",
  appleWebApp: {
    capable: true,
    title: "Ring Promoter",
    statusBarStyle: "black-translucent",
  },
};

// Google tag (gtag.js). Lives in the root layout so every route gets it.
const GA_ID = "G-NM0J18KLK7";

export const viewport: Viewport = {
  themeColor: "#0a0a0a",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${poppins.variable} ${unbounded.variable} ${jetbrainsMono.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <Providers>{children}</Providers>
        <ServiceWorkerRegister />
        <Script
          src={`https://www.googletagmanager.com/gtag/js?id=${GA_ID}`}
          strategy="afterInteractive"
        />
        <Script id="gtag-init" strategy="afterInteractive">
          {`window.dataLayer = window.dataLayer || [];
function gtag(){dataLayer.push(arguments);}
gtag('js', new Date());
gtag('config', '${GA_ID}');`}
        </Script>
      </body>
    </html>
  );
}
