"use client";

import { motion, useReducedMotion } from "motion/react";

/**
 * Restrained in-view reveal, once, honoring reduced motion.
 * - "rise": short fade + rise, for body copy.
 * - "mask": clip reveal from its own baseline, for section headings.
 */
export function Reveal({
  children,
  delay = 0,
  className,
  variant = "rise",
}: {
  children: React.ReactNode;
  delay?: number;
  className?: string;
  variant?: "rise" | "mask";
}) {
  const reduce = useReducedMotion();
  const mask = variant === "mask";
  // `initial` must not depend on `reduce`: it is serialized into the SSR
  // HTML, and branching on a client-only media query mismatches hydration.
  // Reduced motion instead zeroes the transition, so content snaps in place.
  return (
    <motion.div
      className={className}
      initial={
        mask
          ? { opacity: 0, y: 20, clipPath: "inset(0 0 100% 0)" }
          : { opacity: 0, y: 14 }
      }
      whileInView={
        mask
          ? { opacity: 1, y: 0, clipPath: "inset(0 0 -12% 0)" }
          : { opacity: 1, y: 0 }
      }
      viewport={{ once: true, margin: "-60px" }}
      transition={
        reduce
          ? { duration: 0 }
          : { duration: mask ? 0.65 : 0.55, ease: [0.16, 1, 0.3, 1], delay }
      }
    >
      {children}
    </motion.div>
  );
}
