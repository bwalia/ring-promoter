import type { Config } from "tailwindcss";

// Design tokens follow the kubepilot "dark cockpit" language, but as CSS
// variables (defined in src/styles/globals.css) so a light theme can be added
// later without touching component classes.
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        rp: {
          bg: "rgb(var(--rp-bg) / <alpha-value>)",
          surface: "rgb(var(--rp-surface) / <alpha-value>)",
          "surface-2": "rgb(var(--rp-surface-2) / <alpha-value>)",
          border: "rgb(var(--rp-border) / <alpha-value>)",
          "border-hover": "rgb(var(--rp-border-hover) / <alpha-value>)",
          accent: "rgb(var(--rp-accent) / <alpha-value>)",
          "accent-light": "rgb(var(--rp-accent-light) / <alpha-value>)",
          success: "rgb(var(--rp-success) / <alpha-value>)",
          warning: "rgb(var(--rp-warning) / <alpha-value>)",
          danger: "rgb(var(--rp-danger) / <alpha-value>)",
          muted: "rgb(var(--rp-muted) / <alpha-value>)",
          "text-secondary": "rgb(var(--rp-text-secondary) / <alpha-value>)",
        },
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "ui-monospace", "monospace"],
      },
      fontSize: {
        "2xs": ["0.6875rem", { lineHeight: "1rem" }],
      },
      boxShadow: {
        card: "0 1px 3px 0 rgb(0 0 0 / 0.3), 0 1px 2px -1px rgb(0 0 0 / 0.3)",
        "card-hover":
          "0 4px 12px 0 rgb(0 0 0 / 0.4), 0 2px 4px -2px rgb(0 0 0 / 0.4)",
      },
      keyframes: {
        "fade-in": {
          from: { opacity: "0", transform: "translateY(4px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
        "slide-in-right": {
          from: { transform: "translateX(100%)" },
          to: { transform: "translateX(0)" },
        },
        "pulse-soft": {
          "0%, 100%": { opacity: "1" },
          "50%": { opacity: "0.5" },
        },
        "flow-dash": {
          to: { strokeDashoffset: "-16" },
        },
      },
      animation: {
        "fade-in": "fade-in 0.2s ease-out",
        "slide-in-right": "slide-in-right 0.25s ease-out",
        "pulse-soft": "pulse-soft 1.6s ease-in-out infinite",
        "flow-dash": "flow-dash 0.8s linear infinite",
      },
    },
  },
  plugins: [],
} satisfies Config;
