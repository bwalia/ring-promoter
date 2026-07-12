"use client";

// Single home for GSAP registration and the landing page's motion language.
// All landing motion code imports gsap/ScrollTrigger/useGSAP from here (never
// from "gsap" directly) so plugin registration happens exactly once and every
// animation speaks the same physical dialect.
//
// The identity: "proof moves forward; state stays behind." Motion either
// advances (left→right), commits (settles into stillness), or contains
// (snaps back fast, no overshoot). Nothing drifts, nothing loops for decor.

import { gsap } from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { CustomEase } from "gsap/CustomEase";
import { useGSAP } from "@gsap/react";

gsap.registerPlugin(useGSAP, ScrollTrigger, CustomEase);

// GSAP-side registrations of the same curves (usable as ease: "commit" etc.).
CustomEase.create("commit", "0.16, 1, 0.3, 1");
CustomEase.create("advance", "0.65, 0, 0.35, 1");
CustomEase.create("contain", "0.7, 0, 0.84, 0");

// Mobile browser URL-bar show/hide fires resize events that would otherwise
// jump pinned sections mid-scroll.
if (typeof window !== "undefined") {
  ScrollTrigger.config({ ignoreMobileResize: true });
}

/** gsap.matchMedia() query — all motion lives inside this branch, so a
 *  reduced-motion OS setting reverts everything to the static SSR markup. */
export const MOTION_OK = "(prefers-reduced-motion: no-preference)";

/** Named easings (CSS cubic-bezier strings usable by GSAP and motion/react). */
export const EASE = {
  /** entering confirmed state: arrivals, cards settling, ticks landing */
  commit: "cubic-bezier(0.16, 1, 0.3, 1)",
  /** version movement between rings: firm departure, controlled arrival */
  advance: "cubic-bezier(0.65, 0, 0.35, 1)",
  /** failure/rollback: faster than promotion, zero overshoot */
  contain: "cubic-bezier(0.7, 0, 0.84, 0)",
  /** stroke draws, verification sweeps */
  scan: "none",
} as const;

/** Duration scale in seconds (GSAP units). */
export const DUR = {
  tick: 0.15, // status ticks, color commits, check strokes
  settle: 0.28, // cards resolving, log rows, lock acknowledgement
  hop: 0.6, // ring-to-ring movement, section-level state changes
  sequence: 1.2, // a full proof sequence within a section
} as const;

/** Stagger is dependency, not decoration. */
export const STAGGER = 0.1;

export { gsap, ScrollTrigger, useGSAP };
