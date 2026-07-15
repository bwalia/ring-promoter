import type { Metadata } from "next";
import Link from "next/link";
import {
  CalendarOff,
  FileCode2,
  Gauge,
  History,
  MessagesSquare,
  Percent,
  Users,
  Workflow,
} from "lucide-react";
import { AutoPromoteDiagram } from "@/components/landing/autopromote-diagram";
import { CtaTerminal } from "@/components/landing/cta-terminal";
import { DeployerPaths } from "@/components/landing/deployer-paths";
import { FactStrip } from "@/components/landing/fact-strip";
import { ScrollRefresh } from "@/components/landing/gsap/scroll-refresh";
import { HeroSim } from "@/components/landing/hero-sim";
import { JobPanel } from "@/components/landing/job-panel";
import { ProdGateCards } from "@/components/landing/prod-gate-cards";
import { ProtocolSection } from "@/components/landing/protocol-scrub";
import { Reveal } from "@/components/landing/reveal";
import { Spine } from "@/components/landing/spine";
import { YamlPanel } from "@/components/landing/yaml-parse";
import { cn } from "@/lib/utils";

export const metadata: Metadata = {
  title: "Ring Promoter — Every release earns production",
  description:
    "A small control plane that promotes application versions through int → test → acc → prod — health-gated, auto-rolled-back, fully audited. Kubernetes and VM apps, one Go binary.",
};

const GITHUB = "https://github.com/bwalia/ring-promoter";

// The landing page commits to the product's dark control-room look
// (explicit colors, independent of the console's theme toggle).
export default function LandingPage() {
  return (
    <div className="landing relative min-h-screen overflow-x-clip bg-[#0a0c14] text-slate-300 antialiased selection:bg-[#8b83ff]/25 selection:text-white">
      <ScrollRefresh />
      <Spine />
      <Nav />
      <main>
        <Hero />
        <FactStrip />
        <Protocol />
        <AutoPromote />
        <LiveOps />
        <ProdGate />
        <Deployers />
        <Config />
        <Roadmap />
        <Faq />
        <ClosingCta />
      </main>
      <Footer />
    </div>
  );
}

/* ── chrome ──────────────────────────────────────────────────────────── */

function Nav() {
  const links = [
    ["Protocol", "#protocol"],
    ["Live ops", "#live-ops"],
    ["Deployers", "#deployers"],
    ["Config", "#config"],
    ["Roadmap", "#roadmap"],
    ["FAQ", "#faq"],
  ] as const;
  return (
    <header className="sticky top-0 z-40 border-b border-white/[0.07] bg-[#0a0c14]/85 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4 sm:px-6">
        <a href="#" className="flex items-center gap-2.5">
          <RingMark className="size-5" />
          <span className="text-sm font-semibold tracking-tight text-slate-100">
            Ring Promoter
          </span>
        </a>
        <nav aria-label="Sections" className="hidden items-center gap-6 md:flex">
          {links.map(([label, href]) => (
            <a
              key={href}
              href={href}
              className="text-[13px] text-slate-400 transition-colors hover:text-slate-100"
            >
              {label}
            </a>
          ))}
        </nav>
        <div className="flex items-center gap-4">
          <a
            href={GITHUB}
            target="_blank"
            rel="noreferrer"
            className="hidden text-[13px] text-slate-400 transition-colors hover:text-slate-100 sm:block"
          >
            GitHub ↗
          </a>
          <Link
            href="/"
            className="rounded-md bg-[#8b83ff] px-3 py-1.5 text-[13px] font-semibold text-[#0a0c14] transition-colors hover:bg-[#a9a3ff]"
          >
            Open the console
          </Link>
        </div>
      </div>
    </header>
  );
}

// Four arcs = int / test / acc / prod. Each inner arc is fainter; prod is the
// outer, heaviest arc, drawn in iris with a gate notch at the top — the ring a
// release must earn. The dot is the version sitting in production.
function RingMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden className={className}>
      {/* int, test, acc — the inner rings, progressively brighter */}
      <circle cx="12" cy="12" r="4" stroke="currentColor" strokeOpacity="0.3" strokeWidth="1.4" />
      <circle cx="12" cy="12" r="7" stroke="currentColor" strokeOpacity="0.5" strokeWidth="1.4" />
      {/* prod — the earned outer ring, iris, with a gate notch at 12 o'clock */}
      <circle
        cx="12"
        cy="12"
        r="10"
        stroke="#8b83ff"
        strokeWidth="1.8"
        strokeDasharray="60 3.4"
        strokeDashoffset="-1.7"
        transform="rotate(-90 12 12)"
        strokeLinecap="round"
      />
      <circle cx="12" cy="12" r="1.7" fill="#8b83ff" />
    </svg>
  );
}

function Footer() {
  return (
    <footer className="border-t border-white/[0.07]">
      <div className="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-10 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <div className="flex items-center gap-2.5">
          <RingMark className="size-4" />
          <span className="text-sm text-slate-400">
            Ring Promoter — a control plane for release promotion
          </span>
        </div>
        <div className="flex items-center gap-6 text-[13px] text-slate-500">
          <span className="font-mono text-xs">int → test → acc → prod</span>
          <a href={GITHUB} target="_blank" rel="noreferrer" className="transition-colors hover:text-slate-200">
            GitHub ↗
          </a>
          <Link href="/" className="transition-colors hover:text-slate-200">
            Console
          </Link>
        </div>
      </div>
    </footer>
  );
}

/* ── hero ────────────────────────────────────────────────────────────── */

function Hero() {
  return (
    <section className="relative overflow-hidden">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage:
            "linear-gradient(rgba(255,255,255,0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.03) 1px, transparent 1px)",
          backgroundSize: "44px 44px",
          maskImage: "radial-gradient(ellipse 90% 70% at 50% 0%, black 45%, transparent 100%)",
        }}
      />
      <div className="relative mx-auto max-w-6xl px-4 pb-20 pt-20 sm:px-6 sm:pt-28">
        <div className="mx-auto max-w-3xl text-center">
          <p
            className="ls-rise inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1 font-mono text-xs text-slate-400"
            style={{ "--d": "0.3s" } as React.CSSProperties}
          >
            <span className="size-1.5 rounded-full bg-[#8b83ff]" aria-hidden />
            int → test → acc → prod
          </p>
          <h1 className="font-display mt-6 text-4xl font-bold leading-[1.04] tracking-tight text-slate-50 sm:text-[3.6rem]">
            <span className="ls-mask-line">
              <span>Every release</span>
            </span>
            <span className="ls-mask-line" style={{ "--d": "0.12s" } as React.CSSProperties}>
              <span>earns production.</span>
            </span>
          </h1>
          <p
            className="ls-rise mx-auto mt-6 max-w-2xl text-pretty text-base leading-relaxed text-slate-400 sm:text-lg"
            style={{ "--d": "0.4s" } as React.CSSProperties}
          >
            Ring Promoter is a small control plane that moves application versions
            through deployment rings — health-gated at every hop, rolled back
            automatically on failure, and written to history every time.
            Kubernetes and VM apps, one Go binary.
          </p>
          <div
            className="ls-rise mt-8 flex flex-wrap items-center justify-center gap-3"
            style={{ "--d": "0.52s" } as React.CSSProperties}
          >
            <Link
              href="/"
              className="rounded-md bg-[#8b83ff] px-5 py-2.5 text-sm font-semibold text-[#0a0c14] transition-colors hover:bg-[#a9a3ff]"
            >
              Open the console
            </Link>
            <a
              href={`${GITHUB}#how-it-works`}
              target="_blank"
              rel="noreferrer"
              className="rounded-md border border-white/15 px-5 py-2.5 text-sm font-medium text-slate-200 transition-colors hover:bg-white/[0.06]"
            >
              Read the deployment model
            </a>
          </div>
        </div>
        <div className="mx-auto mt-14 max-w-4xl sm:mt-16">
          <HeroSim />
          <p
            className="ls-rise mt-3 text-center font-mono text-[11px] text-slate-600"
            style={{ "--d": "0.8s" } as React.CSSProperties}
          >
            A scripted simulation of the real promotion protocol — try failing a health check.
          </p>
        </div>
      </div>
    </section>
  );
}

/* ── section scaffolding ─────────────────────────────────────────────── */

function SectionHead({
  eyebrow,
  title,
  lede,
  center,
}: {
  eyebrow: string;
  title: string;
  lede?: string;
  center?: boolean;
}) {
  return (
    <Reveal variant="mask" className={cn("max-w-2xl", center && "mx-auto text-center")}>
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-[#8b83ff]/90">
        {eyebrow}
      </p>
      <h2 className="font-display mt-3 text-balance text-3xl font-bold tracking-tight text-slate-50 sm:text-4xl">
        {title}
      </h2>
      {lede && (
        <p className="mt-4 text-pretty leading-relaxed text-slate-400">{lede}</p>
      )}
    </Reveal>
  );
}

/* ── the protocol (pinned scroll-scrubbed stage; see protocol-scrub.tsx) ── */

function Protocol() {
  return <ProtocolSection />;
}

/* ── auto-promote ────────────────────────────────────────────────────── */

function AutoPromote() {
  return (
    <section className="border-y border-white/[0.07] bg-[#0f131e]">
      <div className="mx-auto grid max-w-6xl grid-cols-1 items-center gap-10 px-4 py-16 sm:px-6 lg:grid-cols-2">
        <Reveal>
          <h2 className="font-display text-balance text-2xl font-bold tracking-tight text-slate-50">
            Flow while it&rsquo;s safe. Stop where it matters.
          </h2>
          <p className="mt-4 text-pretty text-[15px] leading-relaxed text-slate-400">
            Flag a ring for auto-promote and a healthy landing continues onward in
            the same operation — hop by hop, under the same lock, with the same
            gates and the same auto-rollback. The chain stops at the first ring
            with the flag off, so nothing reaches production without a human.
          </p>
        </Reveal>
        <AutoPromoteDiagram />
      </div>
    </section>
  );
}

/* ── live operations ─────────────────────────────────────────────────── */

function LiveOps() {
  return (
    <section id="live-ops" className="scroll-mt-20">
      <div className="mx-auto grid max-w-6xl grid-cols-1 items-center gap-12 px-4 py-24 sm:px-6 lg:grid-cols-2">
        <div>
          <SectionHead
            eyebrow="Live operations"
            title="Watch every hop land."
            lede="Seed, promote and rollback run as live jobs. Each step reports status, logs and duration to the console as it happens — acquire the lock, check the source, deploy, verify, record."
          />
          <Reveal delay={0.1}>
            <ul className="mt-8 space-y-4">
              {(
                [
                  [History, "A complete audit trail", "Full history per application and ring — who moved what, where, when, and whether it held."],
                  [Gauge, "Step-level visibility", "The console polls running jobs every second: per-step status, logs and durations, live."],
                  [Workflow, "AI diagnosis on failure", "Failed deploys get an AI diagnosis grounded in the persisted step logs — evidence, not guesswork."],
                ] as const
              ).map(([Icon, title, body]) => (
                <li key={title} className="flex gap-3.5">
                  <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04]">
                    <Icon aria-hidden className="size-4 text-slate-300" />
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold text-slate-100">{title}</h3>
                    <p className="mt-1 text-[13px] leading-relaxed text-slate-500">{body}</p>
                  </div>
                </li>
              ))}
            </ul>
          </Reveal>
        </div>
        <JobPanel />
      </div>
    </section>
  );
}

/* ── prod gate ───────────────────────────────────────────────────────── */

function ProdGate() {
  return (
    <section id="gate" className="scroll-mt-20 border-y border-white/[0.07] bg-[#0f131e]">
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6">
        <SectionHead
          center
          eyebrow="The production gate"
          title="Harder to enter than to leave."
          lede="Production deserves asymmetry: deliberate on the way in, instant on the way out."
        />
        <ProdGateCards />
      </div>
    </section>
  );
}

/* ── deployers ───────────────────────────────────────────────────────── */

function Deployers() {
  return (
    <section id="deployers" className="scroll-mt-20">
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6">
        <SectionHead
          eyebrow="Deployers"
          title="It meets your infrastructure where it is."
          lede="The deployer is chosen per application — one control plane promotes Kubernetes services and VM apps side by side, under the same rules."
        />
        <DeployerPaths />
      </div>
    </section>
  );
}

/* ── config ──────────────────────────────────────────────────────────── */

function Config() {
  return (
    <section id="config" className="scroll-mt-20 border-y border-white/[0.07] bg-[#0f131e]">
      <div className="mx-auto grid max-w-6xl grid-cols-1 items-center gap-12 px-4 py-24 sm:px-6 lg:grid-cols-2">
        <div>
          <SectionHead
            eyebrow="Onboarding"
            title="An app is a block of YAML."
            lede="No plugin, no SDK, no rebuild. Declare where each ring lives and how to check its health, apply the config, and the app appears in the console. Apps only define the rings they actually live in."
          />
        </div>
        <YamlPanel />
      </div>
    </section>
  );
}

/* ── roadmap ─────────────────────────────────────────────────────────── */

function Roadmap() {
  const items = [
    [MessagesSquare, "Chat-ops approvals", "Approve a gated prod promotion from Slack or Teams — with the audit entry recording who.", "planned"],
    [Gauge, "Metric-based gates", "Gate promotions on Prometheus / OpenTelemetry queries — error rate and latency, not just a 200 from /health.", "planned"],
    [CalendarOff, "Freeze windows", "Block promotions during change freezes and out-of-hours. Rollbacks stay exempt, always.", "planned"],
    [Percent, "Canary steps inside a ring", "Shift a slice of traffic to the new version before the ring flips over.", "exploring"],
    [Users, "SSO & per-app roles", "OIDC sign-in with per-app permissions, so every approval carries an identity.", "exploring"],
    [FileCode2, "GitOps & Terraform", "Declare apps, rings and gates from your repository instead of a ConfigMap.", "exploring"],
  ] as const;
  return (
    <section id="roadmap" className="scroll-mt-20">
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6">
        <SectionHead
          eyebrow="Roadmap"
          title="Where the ring is heading."
          lede="Not shipped yet — this is the direction the control plane is growing, ordered by what platform teams keep asking for. Open a GitHub issue to shape the priority."
        />
        <div className="mt-12 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {items.map(([Icon, title, body, stage], i) => (
            <Reveal key={title} delay={i * 0.05}>
              {/* exploring stays dashed — the future is visibly uncommitted */}
              <div
                className={cn(
                  "h-full rounded-xl border bg-[#0f131e] p-5",
                  stage === "exploring"
                    ? "border-dashed border-white/[0.13]"
                    : "border-white/[0.07]",
                )}
              >
                <div className="flex items-center justify-between">
                  <span className="flex size-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04]">
                    <Icon aria-hidden className="size-4 text-slate-300" />
                  </span>
                  <span
                    className={cn(
                      "rounded-full border px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider",
                      stage === "planned"
                        ? "border-[#8b83ff]/25 text-[#a9a3ff]/90"
                        : "border-white/15 text-slate-500",
                    )}
                  >
                    {stage}
                  </span>
                </div>
                <h3 className="mt-4 text-sm font-semibold text-slate-100">{title}</h3>
                <p className="mt-2 text-[13px] leading-relaxed text-slate-500">{body}</p>
              </div>
            </Reveal>
          ))}
        </div>
      </div>
    </section>
  );
}

/* ── faq ─────────────────────────────────────────────────────────────── */

function Faq() {
  const qas = [
    [
      "Does it replace my CI?",
      "No. CI builds and tests; Ring Promoter moves what CI produced through your environments. A pipeline calls one authenticated endpoint — seed or promote — and a non-2xx response means the promotion failed, so `curl --fail` is all the integration you need.",
    ],
    [
      "What do I need to run it?",
      "One small binary (or container) — the web console and REST API are embedded in it. Locally it runs with an in-memory store and a no-op deployer, no cluster or database needed. In production: Postgres and, for Kubernetes apps, a ServiceAccount.",
    ],
    [
      "What counts as a version?",
      "For Kubernetes apps, an image tag. For GitHub-deployed apps, a branch, tag or commit SHA — validated against the repository before anything is dispatched, and the console's picker only offers refs that exist.",
    ],
    [
      "What happens when a health check fails?",
      "The target ring is retried a configurable number of times, then automatically rolled back to its previous version. The failure, the rollback and the step logs all land in history, and the console can produce an AI diagnosis from that evidence.",
    ],
    [
      "Can two promotions collide?",
      "No. Operations on the same application are serialized by a Postgres advisory lock, which holds across replicas — an accidental scale-up cannot run two concurrent promotions on the same app.",
    ],
  ] as const;
  return (
    <section id="faq" className="scroll-mt-20 border-t border-white/[0.07]">
      <div className="mx-auto max-w-3xl px-4 py-24 sm:px-6">
        <SectionHead center eyebrow="FAQ" title="Fair questions." />
        <Reveal delay={0.1}>
          <div className="mt-10 divide-y divide-white/[0.07] rounded-xl border border-white/[0.07] bg-[#0f131e]">
            {qas.map(([q, a]) => (
              <details key={q} className="ls-faq group px-5 py-4 open:pb-5">
                <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-sm font-medium text-slate-200 [&::-webkit-details-marker]:hidden">
                  {q}
                  <span
                    aria-hidden
                    className="text-slate-600 transition-transform duration-200 group-open:rotate-45"
                  >
                    +
                  </span>
                </summary>
                <p className="mt-3 text-sm leading-relaxed text-slate-500">{a}</p>
              </details>
            ))}
          </div>
        </Reveal>
      </div>
    </section>
  );
}

/* ── closing ─────────────────────────────────────────────────────────── */

function ClosingCta() {
  return (
    <section id="cta" className="relative overflow-hidden border-t border-white/[0.07]">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage:
            "linear-gradient(rgba(255,255,255,0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.03) 1px, transparent 1px)",
          backgroundSize: "44px 44px",
          maskImage: "radial-gradient(ellipse 80% 90% at 50% 100%, black 40%, transparent 100%)",
        }}
      />
      <div className="relative mx-auto max-w-3xl px-4 py-24 text-center sm:px-6">
        <Reveal>
          <h2 className="font-display text-balance text-3xl font-bold tracking-tight text-slate-50 sm:text-4xl">
            Bring release discipline to your cluster.
          </h2>
          <p className="mx-auto mt-4 max-w-xl text-pretty leading-relaxed text-slate-400">
            Try it in one command — the defaults use an in-memory store and a
            no-op deployer, so there is nothing to install and nothing to break.
          </p>
          <CtaTerminal github={GITHUB} />
        </Reveal>
      </div>
    </section>
  );
}
