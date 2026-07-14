/**
 * The Ring Promoter mark: four arcs for int / test / acc / prod. The inner
 * rings are fainter; prod is the outer, heaviest arc with a gate notch at the
 * top — the ring a release must earn. Monochrome (currentColor) so it inherits
 * whatever it sits in; the landing page uses its own iris-tinted variant.
 */
export function RingMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden className={className}>
      <circle cx="12" cy="12" r="4" stroke="currentColor" strokeOpacity="0.4" strokeWidth="1.4" />
      <circle cx="12" cy="12" r="7" stroke="currentColor" strokeOpacity="0.65" strokeWidth="1.4" />
      <circle
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeDasharray="60 3.4"
        strokeDashoffset="-1.7"
        transform="rotate(-90 12 12)"
        strokeLinecap="round"
      />
      <circle cx="12" cy="12" r="1.7" fill="currentColor" />
    </svg>
  );
}
