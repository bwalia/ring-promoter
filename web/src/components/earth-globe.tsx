"use client";

import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";
import { EARTH_R, LAND_POLYS, projectOrtho } from "@/lib/globe-layout";
import { SOLAR_C } from "@/lib/solar-layout";

export type EarthGlobeHandle = {
  setSpin: (rad: number) => void;
};

/**
 * Orthographic Earth drawn in canvas. The parent drives spin via the handle
 * so a rAF loop can rotate the globe without re-rendering React.
 */
export const EarthGlobe = forwardRef<EarthGlobeHandle, { className?: string }>(
  function EarthGlobe({ className }, ref) {
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const spinRef = useRef(0);
    const dirty = useRef(true);

    const paint = () => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      const ctx = canvas.getContext("2d");
      if (!ctx) return;

      const dpr = Math.min(2, window.devicePixelRatio || 1);
      const css = canvas.clientWidth || 400;
      const size = Math.round(css * dpr);
      if (canvas.width !== size || canvas.height !== size) {
        canvas.width = size;
        canvas.height = size;
      }
      const scale = size / 400;
      ctx.setTransform(scale, 0, 0, scale, 0, 0);
      ctx.clearRect(0, 0, 400, 400);

      const spin = spinRef.current;
      const cx = SOLAR_C;
      const cy = SOLAR_C;

      // Atmosphere
      const atm = ctx.createRadialGradient(cx, cy, EARTH_R * 0.92, cx, cy, EARTH_R * 1.28);
      atm.addColorStop(0, "rgba(56, 189, 248, 0)");
      atm.addColorStop(0.72, "rgba(56, 189, 248, 0.07)");
      atm.addColorStop(1, "rgba(56, 189, 248, 0)");
      ctx.fillStyle = atm;
      ctx.beginPath();
      ctx.arc(cx, cy, EARTH_R * 1.28, 0, Math.PI * 2);
      ctx.fill();

      ctx.save();
      ctx.beginPath();
      ctx.arc(cx, cy, EARTH_R, 0, Math.PI * 2);
      ctx.clip();

      // Ocean
      const ocean = ctx.createRadialGradient(
        cx - EARTH_R * 0.28,
        cy - EARTH_R * 0.34,
        EARTH_R * 0.08,
        cx,
        cy,
        EARTH_R,
      );
      ocean.addColorStop(0, "#1c4a6e");
      ocean.addColorStop(0.45, "#0d2a44");
      ocean.addColorStop(1, "#07141f");
      ctx.fillStyle = ocean;
      ctx.fillRect(cx - EARTH_R, cy - EARTH_R, EARTH_R * 2, EARTH_R * 2);

      // Graticule
      ctx.strokeStyle = "rgba(186, 230, 253, 0.11)";
      ctx.lineWidth = 0.55;
      for (let lng = -180; lng < 180; lng += 30) {
        strokeMeridian(ctx, lng, spin);
      }
      for (let lat = -60; lat <= 60; lat += 30) {
        strokeParallel(ctx, lat, spin);
      }

      // Land
      ctx.fillStyle = "rgba(134, 168, 128, 0.78)";
      ctx.strokeStyle = "rgba(190, 210, 170, 0.18)";
      ctx.lineWidth = 0.4;
      for (const poly of LAND_POLYS) {
        drawLand(ctx, poly, spin);
      }

      // Terminator / night side
      const night = ctx.createLinearGradient(cx - EARTH_R, cy, cx + EARTH_R, cy);
      night.addColorStop(0, "rgba(2, 6, 12, 0.22)");
      night.addColorStop(0.42, "rgba(2, 6, 12, 0)");
      night.addColorStop(0.62, "rgba(2, 6, 12, 0)");
      night.addColorStop(1, "rgba(2, 6, 12, 0.55)");
      ctx.fillStyle = night;
      ctx.fillRect(cx - EARTH_R, cy - EARTH_R, EARTH_R * 2, EARTH_R * 2);

      // Specular
      const spec = ctx.createRadialGradient(
        cx - EARTH_R * 0.32,
        cy - EARTH_R * 0.4,
        0,
        cx - EARTH_R * 0.32,
        cy - EARTH_R * 0.4,
        EARTH_R * 0.55,
      );
      spec.addColorStop(0, "rgba(255, 255, 255, 0.22)");
      spec.addColorStop(0.35, "rgba(186, 230, 253, 0.06)");
      spec.addColorStop(1, "rgba(255, 255, 255, 0)");
      ctx.fillStyle = spec;
      ctx.beginPath();
      ctx.arc(cx - EARTH_R * 0.32, cy - EARTH_R * 0.4, EARTH_R * 0.55, 0, Math.PI * 2);
      ctx.fill();

      ctx.restore();

      // Limb
      ctx.beginPath();
      ctx.arc(cx, cy, EARTH_R, 0, Math.PI * 2);
      ctx.strokeStyle = "rgba(125, 211, 252, 0.28)";
      ctx.lineWidth = 1.1;
      ctx.stroke();
      ctx.beginPath();
      ctx.arc(cx, cy, EARTH_R + 1.6, 0, Math.PI * 2);
      ctx.strokeStyle = "rgba(245, 185, 66, 0.12)";
      ctx.lineWidth = 0.7;
      ctx.stroke();
    };

    useImperativeHandle(ref, () => ({
      setSpin(rad: number) {
        spinRef.current = rad;
        dirty.current = true;
      },
    }));

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
      // paint reads canvas + spin; we intentionally don't list it.
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
  },
);

function strokeMeridian(
  ctx: CanvasRenderingContext2D,
  lng: number,
  spin: number,
) {
  ctx.beginPath();
  let started = false;
  for (let lat = -90; lat <= 90; lat += 4) {
    const p = projectOrtho(lat, lng, EARTH_R, spin);
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
) {
  ctx.beginPath();
  let started = false;
  for (let lng = -180; lng <= 180; lng += 5) {
    const p = projectOrtho(lat, lng, EARTH_R, spin);
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
) {
  const pts = poly.map(([lng, lat]) => projectOrtho(lat, lng, EARTH_R, spin));
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
