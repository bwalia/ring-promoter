"use client";

import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";
import {
  DEFAULT_METRICS,
  LAND_POLYS,
  projectOrtho,
  type GlobeMetrics,
} from "@/lib/globe-layout";

export type EarthGlobeHandle = {
  setSpin: (rad: number) => void;
};

/**
 * Orthographic Earth drawn in canvas. The parent drives spin via the handle
 * so a rAF loop can rotate the globe without re-rendering React. Geometry
 * comes from live `GlobeMetrics` so Earth grows with the stage.
 */
export const EarthGlobe = forwardRef<
  EarthGlobeHandle,
  { className?: string; metrics?: GlobeMetrics }
>(function EarthGlobe({ className, metrics = DEFAULT_METRICS }, ref) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const spinRef = useRef(0);
  const metricsRef = useRef(metrics);
  metricsRef.current = metrics;
  const dirty = useRef(true);

  const paint = () => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const m = metricsRef.current;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const cssW = canvas.clientWidth || m.width;
    const cssH = canvas.clientHeight || m.height;
    const bufW = Math.max(1, Math.round(cssW * dpr));
    const bufH = Math.max(1, Math.round(cssH * dpr));
    if (canvas.width !== bufW || canvas.height !== bufH) {
      canvas.width = bufW;
      canvas.height = bufH;
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, cssW, cssH);

    const spin = spinRef.current;
    const cx = m.cx;
    const cy = m.cy;
    const earthR = m.earthR;
    const strokeK = Math.min(cssW, cssH) / 400;

    // Atmosphere halo — slightly stronger for a cinematic limb without
    // competing with the emerald console chrome.
    const atm = ctx.createRadialGradient(cx, cy, earthR * 0.9, cx, cy, earthR * 1.36);
    atm.addColorStop(0, "rgba(56, 189, 248, 0)");
    atm.addColorStop(0.65, "rgba(56, 189, 248, 0.05)");
    atm.addColorStop(0.88, "rgba(125, 211, 252, 0.14)");
    atm.addColorStop(1, "rgba(56, 189, 248, 0)");
    ctx.fillStyle = atm;
    ctx.beginPath();
    ctx.arc(cx, cy, earthR * 1.36, 0, Math.PI * 2);
    ctx.fill();

    ctx.save();
    ctx.beginPath();
    ctx.arc(cx, cy, earthR, 0, Math.PI * 2);
    ctx.clip();

    // Ocean
    const ocean = ctx.createRadialGradient(
      cx - earthR * 0.28,
      cy - earthR * 0.34,
      earthR * 0.08,
      cx,
      cy,
      earthR,
    );
    ocean.addColorStop(0, "#1c4a6e");
    ocean.addColorStop(0.45, "#0d2a44");
    ocean.addColorStop(1, "#07141f");
    ctx.fillStyle = ocean;
    ctx.fillRect(cx - earthR, cy - earthR, earthR * 2, earthR * 2);

    // Graticule
    ctx.strokeStyle = "rgba(186, 230, 253, 0.11)";
    ctx.lineWidth = 0.55 * strokeK;
    for (let lng = -180; lng < 180; lng += 30) {
      strokeMeridian(ctx, lng, spin, m);
    }
    for (let lat = -60; lat <= 60; lat += 30) {
      strokeParallel(ctx, lat, spin, m);
    }

    // Land
    ctx.fillStyle = "rgba(148, 180, 138, 0.86)";
    ctx.strokeStyle = "rgba(210, 225, 190, 0.22)";
    ctx.lineWidth = 0.45 * strokeK;
    for (const poly of LAND_POLYS) {
      drawLand(ctx, poly, spin, m);
    }

    // Terminator / night side
    const night = ctx.createLinearGradient(cx - earthR, cy, cx + earthR, cy);
    night.addColorStop(0, "rgba(2, 6, 12, 0.22)");
    night.addColorStop(0.42, "rgba(2, 6, 12, 0)");
    night.addColorStop(0.62, "rgba(2, 6, 12, 0)");
    night.addColorStop(1, "rgba(2, 6, 12, 0.55)");
    ctx.fillStyle = night;
    ctx.fillRect(cx - earthR, cy - earthR, earthR * 2, earthR * 2);

    // Specular
    const spec = ctx.createRadialGradient(
      cx - earthR * 0.32,
      cy - earthR * 0.4,
      0,
      cx - earthR * 0.32,
      cy - earthR * 0.4,
      earthR * 0.55,
    );
    spec.addColorStop(0, "rgba(255, 255, 255, 0.22)");
    spec.addColorStop(0.35, "rgba(186, 230, 253, 0.06)");
    spec.addColorStop(1, "rgba(255, 255, 255, 0)");
    ctx.fillStyle = spec;
    ctx.beginPath();
    ctx.arc(cx - earthR * 0.32, cy - earthR * 0.4, earthR * 0.55, 0, Math.PI * 2);
    ctx.fill();

    ctx.restore();

    // Limb — cool rim + faint sun-warm secondary ring
    ctx.beginPath();
    ctx.arc(cx, cy, earthR, 0, Math.PI * 2);
    ctx.strokeStyle = "rgba(186, 230, 253, 0.38)";
    ctx.lineWidth = 1.25 * strokeK;
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(cx, cy, earthR + 2.1 * strokeK, 0, Math.PI * 2);
    ctx.strokeStyle = "rgba(245, 185, 66, 0.16)";
    ctx.lineWidth = 0.8 * strokeK;
    ctx.stroke();
  };

  useImperativeHandle(ref, () => ({
    setSpin(rad: number) {
      spinRef.current = rad;
      dirty.current = true;
    },
  }));

  useEffect(() => {
    dirty.current = true;
  }, [metrics]);

  useEffect(() => {
    let id = 0;
    const tick = () => {
      if (dirty.current) {
        dirty.current = false;
        paint();
      }
      id = requestAnimationFrame(tick);
    };
    dirty.current = true;
    id = requestAnimationFrame(tick);
    const ro = new ResizeObserver(() => {
      dirty.current = true;
    });
    if (canvasRef.current) ro.observe(canvasRef.current);
    return () => {
      cancelAnimationFrame(id);
      ro.disconnect();
    };
    // paint reads canvas + spin + metricsRef; we intentionally don't list it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className={className}
      aria-hidden
      data-earth-globe
    />
  );
});

function strokeMeridian(
  ctx: CanvasRenderingContext2D,
  lng: number,
  spin: number,
  metrics: GlobeMetrics,
) {
  ctx.beginPath();
  let started = false;
  for (let lat = -90; lat <= 90; lat += 4) {
    const p = projectOrtho(lat, lng, metrics.earthR, spin, metrics);
    if (!p.front) {
      started = false;
      continue;
    }
    if (!started) {
      ctx.moveTo(p.x, p.y);
      started = true;
    } else {
      ctx.lineTo(p.x, p.y);
    }
  }
  ctx.stroke();
}

function strokeParallel(
  ctx: CanvasRenderingContext2D,
  lat: number,
  spin: number,
  metrics: GlobeMetrics,
) {
  ctx.beginPath();
  let started = false;
  for (let lng = -180; lng <= 180; lng += 5) {
    const p = projectOrtho(lat, lng, metrics.earthR, spin, metrics);
    if (!p.front) {
      started = false;
      continue;
    }
    if (!started) {
      ctx.moveTo(p.x, p.y);
      started = true;
    } else {
      ctx.lineTo(p.x, p.y);
    }
  }
  ctx.stroke();
}

function drawLand(
  ctx: CanvasRenderingContext2D,
  poly: [number, number][],
  spin: number,
  metrics: GlobeMetrics,
) {
  const pts = poly.map(([lng, lat]) =>
    projectOrtho(lat, lng, metrics.earthR, spin, metrics),
  );
  const front = pts.filter((p) => p.front);
  if (front.length < 3) return;
  ctx.beginPath();
  let started = false;
  for (const p of pts) {
    if (!p.front) {
      started = false;
      continue;
    }
    if (!started) {
      ctx.moveTo(p.x, p.y);
      started = true;
    } else {
      ctx.lineTo(p.x, p.y);
    }
  }
  ctx.closePath();
  ctx.fill();
  ctx.stroke();
}
