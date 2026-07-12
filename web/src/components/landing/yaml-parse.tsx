"use client";

import { useRef } from "react";
import { gsap, useGSAP, MOTION_OK } from "./gsap/setup";

// "Parsed, not typed": the YAML resolves in three deliberate stamps — app
// identity, ring placement, health criteria — each flashing a brief parse
// bar, then an "applied ✓" chip lands in the tab bar. Explicitly no
// keystroke theater. Static markup is the fully-applied file.

function Y({ k, v }: { k: string; v?: string }) {
  return (
    <>
      <span className="text-neutral-400">{k}:</span>
      {v && <span className="text-neutral-100"> {v}</span>}
    </>
  );
}

function C({ t }: { t: string }) {
  return <span className="text-neutral-600">{t}</span>;
}

export function YamlPanel() {
  const scope = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const q = gsap.utils.selector(scope);
      const mm = gsap.matchMedia();
      mm.add(MOTION_OK, () => {
        gsap.set(q(".yp-g"), { opacity: 0.32 });
        gsap.set(q(".yp-applied"), { opacity: 0, scale: 0.8, transformOrigin: "center" });

        const tl = gsap.timeline({
          defaults: { ease: "commit" },
          scrollTrigger: { trigger: scope.current, start: "top 75%", once: true },
        });
        for (let i = 0; i < 3; i++) {
          const at = 0.15 + i * 0.4;
          tl.to(q(`.yp-g-${i}`), { opacity: 1, duration: 0.3 }, at);
          tl.fromTo(
            q(`.yp-g-${i}`),
            { borderLeftColor: "rgba(52,211,153,0.6)" },
            { borderLeftColor: "rgba(52,211,153,0)", duration: 0.55, immediateRender: false },
            at,
          );
        }
        tl.to(q(".yp-applied"), { opacity: 1, scale: 1, duration: 0.3 }, 1.45);
      });
    },
    { scope },
  );

  return (
    <div ref={scope} className="overflow-hidden rounded-xl border border-white/10 bg-[#090909]">
      <div className="flex items-center justify-between border-b border-white/[0.07] px-4 py-2 font-mono text-[11px] text-neutral-500">
        config.yaml
        <span className="yp-applied rounded-full border border-emerald-500/30 px-2 py-0.5 text-[10px] text-emerald-400">
          applied ✓
        </span>
      </div>
      <pre className="overflow-x-auto p-4 font-mono text-xs leading-relaxed">
        <code>
          <span className="yp-g yp-g-0 block border-l-2 border-transparent pl-3 -ml-3">
            <Y k="apps" />
            {"\n"}
            {"  - "}
            <Y k="name" v="billing-worker" />
          </span>
          <span className="yp-g yp-g-1 block border-l-2 border-transparent pl-3 -ml-3">
            {"    "}
            <Y k="rings" />
            {"\n"}
            {"      "}
            <Y k="int" />
            {"\n"}
            {"        "}
            <Y k="namespace" v="int" />
            {"\n"}
            {"        "}
            <Y k="deployment" v="billing-worker" />
            {"\n"}
            {"        "}
            <Y k="container" v="worker" />
            {"\n"}
            {"        "}
            <Y k="image" v="registry.example.com/billing-worker" />
          </span>
          <span className="yp-g yp-g-2 block border-l-2 border-transparent pl-3 -ml-3">
            {"        "}
            <Y k="health_url" v="http://billing-worker.int.svc/health" />
            {"\n"}
            {"      "}
            <Y k="test" />
            {"\n"}
            {"        "}
            <C t="# …same shape, per ring" />
            {"\n"}
            {"      "}
            <Y k="prod" />
            {"\n"}
            {"        "}
            <C t="# only the rings the app lives in" />
          </span>
        </code>
      </pre>
    </div>
  );
}
