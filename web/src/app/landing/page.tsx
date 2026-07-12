import type { Metadata } from "next";
import Link from "next/link";
import {
  CalendarOff,
  Container,
  FileCode2,
  Gauge,
  History,
  Lock,
  MessagesSquare,
  Percent,
  RotateCcw,
  Users,
  Workflow,
} from "lucide-react";
import { HeroSim } from "@/components/landing/hero-sim";
import { Reveal } from "@/components/landing/reveal";
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
    <div className="min-h-screen overflow-x-clip bg-[#090909] text-neutral-300 antialiased selection:bg-emerald-500/25 selection:text-white">
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
    <header className="sticky top-0 z-40 border-b border-white/[0.07] bg-[#090909]/85 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4 sm:px-6">
        <a href="#" className="flex items-center gap-2.5">
          <RingMark className="size-5" />
          <span className="text-sm font-semibold tracking-tight text-neutral-100">
            Ring Promoter
          </span>
        </a>
        <nav aria-label="Sections" className="hidden items-center gap-6 md:flex">
          {links.map(([label, href]) => (
            <a
              key={href}
              href={href}
              className="text-[13px] text-neutral-400 transition-colors hover:text-neutral-100"
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
            className="hidden text-[13px] text-neutral-400 transition-colors hover:text-neutral-100 sm:block"
          >
            GitHub ↗
          </a>
          <Link
            href="/"
            className="rounded-md bg-neutral-100 px-3 py-1.5 text-[13px] font-medium text-neutral-900 transition-colors hover:bg-white"
          >
            Open the console
          </Link>
        </div>
      </div>
    </header>
  );
}

function RingMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden className={className}>
      <circle cx="12" cy="12" r="10" stroke="#22c55e" strokeWidth="1.5" strokeDasharray="2 3.2" />
      <circle cx="12" cy="12" r="5.5" stroke="#e5e5e5" strokeWidth="1.5" />
      <circle cx="12" cy="12" r="1.6" fill="#e5e5e5" />
    </svg>
  );
}

function Footer() {
  return (
    <footer className="border-t border-white/[0.07]">
      <div className="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-10 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <div className="flex items-center gap-2.5">
          <RingMark className="size-4" />
          <span className="text-sm text-neutral-400">
            Ring Promoter — a control plane for release promotion
          </span>
        </div>
        <div className="flex items-center gap-6 text-[13px] text-neutral-500">
          <span className="font-mono text-xs">int → test → acc → prod</span>
          <a href={GITHUB} target="_blank" rel="noreferrer" className="transition-colors hover:text-neutral-200">
            GitHub ↗
          </a>
          <Link href="/" className="transition-colors hover:text-neutral-200">
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
          <p className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1 font-mono text-xs text-neutral-400">
            <span className="size-1.5 rounded-full bg-emerald-500" aria-hidden />
            int → test → acc → prod
          </p>
          <h1 className="mt-6 text-balance text-4xl font-semibold leading-[1.06] tracking-tight text-neutral-50 sm:text-6xl">
            Every release earns production.
          </h1>
          <p className="mx-auto mt-6 max-w-2xl text-pretty text-base leading-relaxed text-neutral-400 sm:text-lg">
            Ring Promoter is a small control plane that moves application versions
            through deployment rings — health-gated at every hop, rolled back
            automatically on failure, and written to history every time.
            Kubernetes and VM apps, one Go binary.
          </p>
          <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
            <Link
              href="/"
              className="rounded-md bg-neutral-100 px-5 py-2.5 text-sm font-medium text-neutral-900 transition-colors hover:bg-white"
            >
              Open the console
            </Link>
            <a
              href={`${GITHUB}#how-it-works`}
              target="_blank"
              rel="noreferrer"
              className="rounded-md border border-white/15 px-5 py-2.5 text-sm font-medium text-neutral-200 transition-colors hover:bg-white/[0.06]"
            >
              Read the deployment model
            </a>
          </div>
        </div>
        <div className="mx-auto mt-14 max-w-4xl sm:mt-16">
          <HeroSim />
          <p className="mt-3 text-center font-mono text-[11px] text-neutral-600">
            A scripted simulation of the real promotion protocol — try failing a health check.
          </p>
        </div>
      </div>
    </section>
  );
}

/* ── fact strip ──────────────────────────────────────────────────────── */

function FactStrip() {
  const facts = [
    ["Single Go binary", "UI, API and promoter in one process"],
    ["Kubernetes + VMs", "kubectl and GitHub Actions, side by side"],
    ["Safe across replicas", "Postgres advisory locks serialize every op"],
    ["Onboard in YAML", "new apps are configuration, not code"],
  ] as const;
  return (
    <section className="border-y border-white/[0.07]">
      <div className="mx-auto grid max-w-6xl grid-cols-2 divide-white/[0.07] max-lg:gap-px lg:grid-cols-4 lg:divide-x">
        {facts.map(([title, sub]) => (
          <div key={title} className="px-4 py-6 sm:px-6">
            <p className="font-mono text-[11px] uppercase tracking-widest text-neutral-500">
              {title}
            </p>
            <p className="mt-1.5 text-sm text-neutral-300">{sub}</p>
          </div>
        ))}
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
    <Reveal className={cn("max-w-2xl", center && "mx-auto text-center")}>
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-emerald-500/90">
        {eyebrow}
      </p>
      <h2 className="mt-3 text-balance text-3xl font-semibold tracking-tight text-neutral-50 sm:text-4xl">
        {title}
      </h2>
      {lede && (
        <p className="mt-4 text-pretty leading-relaxed text-neutral-400">{lede}</p>
      )}
    </Reveal>
  );
}

/* ── the protocol ────────────────────────────────────────────────────── */

function Protocol() {
  const rules = [
    ["One ring at a time", "Promote always targets the next ring in the pipeline. Skipping is impossible by design — not by policy."],
    ["The source must prove it", "A live health check on the source ring gates every promotion before anything deploys."],
    ["The target must earn it", "After deploying, health checks run with configurable retries before the hop is called good."],
    ["Failure undoes itself", "If the target stays unhealthy, it is rolled back to its previous version automatically."],
    ["Everything is written down", "Every seed, promote and rollback lands in history — success or failure, per app, per ring."],
  ] as const;
  return (
    <section id="protocol" className="scroll-mt-20">
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6">
        <SectionHead
          eyebrow="The protocol"
          title="Promotion is a protocol, not a script."
          lede="Five rules, enforced by the control plane on every operation. They are what make a promotion safe to run at 5pm on a Friday."
        />
        <div className="mt-12 grid grid-cols-1 gap-px overflow-hidden rounded-xl border border-white/[0.07] bg-white/[0.07] sm:grid-cols-2 lg:grid-cols-5">
          {rules.map(([title, body], i) => (
            <Reveal key={title} delay={i * 0.05} className="bg-[#0b0b0c] p-5">
              <p className="font-mono text-xs text-emerald-500/80">
                {String(i + 1).padStart(2, "0")}
              </p>
              <h3 className="mt-3 text-sm font-semibold text-neutral-100">{title}</h3>
              <p className="mt-2 text-[13px] leading-relaxed text-neutral-500">{body}</p>
            </Reveal>
          ))}
        </div>
      </div>
    </section>
  );
}

/* ── auto-promote ────────────────────────────────────────────────────── */

function AutoPromote() {
  return (
    <section className="border-y border-white/[0.07] bg-[#0b0b0c]">
      <div className="mx-auto grid max-w-6xl grid-cols-1 items-center gap-10 px-4 py-16 sm:px-6 lg:grid-cols-2">
        <Reveal>
          <h2 className="text-balance text-2xl font-semibold tracking-tight text-neutral-50">
            Flow while it&rsquo;s safe. Stop where it matters.
          </h2>
          <p className="mt-4 text-pretty text-[15px] leading-relaxed text-neutral-400">
            Flag a ring for auto-promote and a healthy landing continues onward in
            the same operation — hop by hop, under the same lock, with the same
            gates and the same auto-rollback. The chain stops at the first ring
            with the flag off, so nothing reaches production without a human.
          </p>
        </Reveal>
        <Reveal delay={0.1}>
          <div className="overflow-x-auto rounded-xl border border-white/[0.07] bg-[#090909] px-4 py-6 font-mono text-xs sm:px-5">
            <div className="flex min-w-[360px] items-center justify-between gap-1">
              {(
                [
                  ["int", "auto", true],
                  ["test", "auto", true],
                  ["acc", "hold", false],
                  ["prod", null, false],
                ] as const
              ).map(([ring, gate, on]) => (
                <div key={ring} className="flex flex-1 items-center gap-1 last:flex-none">
                  <span className="rounded-md border border-white/15 bg-white/[0.05] px-1.5 py-1.5 text-neutral-200 sm:px-2.5">
                    {ring}
                  </span>
                  {gate && (
                    <span className="flex flex-1 items-center gap-1.5 px-1">
                      <span className={cn("h-px flex-1", on ? "bg-emerald-500/50" : "bg-white/15")} />
                      <span
                        className={cn(
                          "flex items-center gap-1 rounded-full border px-1.5 py-0.5 text-[10px]",
                          on
                            ? "border-emerald-500/30 text-emerald-400"
                            : "border-amber-500/30 text-amber-400",
                        )}
                      >
                        <span
                          className={cn("size-1 rounded-full", on ? "bg-emerald-400" : "bg-amber-400")}
                        />
                        {gate}
                      </span>
                      <span className={cn("h-px flex-1", on ? "bg-emerald-500/50" : "bg-white/15")} />
                    </span>
                  )}
                </div>
              ))}
            </div>
            <p className="mt-4 text-[11px] leading-relaxed text-neutral-600">
              # a healthy int → test carries on to acc automatically;{" "}
              <span className="text-amber-500/80">acc holds for a human</span> before prod
            </p>
          </div>
        </Reveal>
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
                    <Icon aria-hidden className="size-4 text-neutral-300" />
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold text-neutral-100">{title}</h3>
                    <p className="mt-1 text-[13px] leading-relaxed text-neutral-500">{body}</p>
                  </div>
                </li>
              ))}
            </ul>
          </Reveal>
        </div>
        <Reveal delay={0.15}>
          <JobPanel />
        </Reveal>
      </div>
    </section>
  );
}

function JobPanel() {
  const steps = [
    ["done", "acquire app lock", "0.0s"],
    ["done", "source health check — int", "0.3s"],
    ["done", "deploy v2.14.0 to test", "8.2s"],
    ["run", "health check test — attempt 2/3", "4.1s"],
    ["todo", "record history", "—"],
  ] as const;
  return (
    <div className="overflow-hidden rounded-2xl border border-white/10 bg-[#0b0b0c] shadow-[0_30px_80px_-40px_rgba(0,0,0,0.9)]">
      <div className="flex items-center justify-between border-b border-white/[0.07] px-4 py-2.5 font-mono text-xs text-neutral-500">
        <span>
          <span className="text-neutral-300">PROMOTE</span> payments-api · int → test
        </span>
        <span>job 8f3a…c2</span>
      </div>
      <ul className="space-y-1 px-4 py-4 font-mono text-xs">
        {steps.map(([state, label, dur]) => (
          <li key={label} className="flex items-center gap-3 py-1">
            {state === "done" ? (
              <span className="flex size-4 items-center justify-center rounded-full bg-emerald-500/15 text-[10px] text-emerald-400">
                ✓
              </span>
            ) : state === "run" ? (
              <span className="relative flex size-4 items-center justify-center">
                <span className="absolute inset-0 animate-ping rounded-full bg-sky-500/40" />
                <span className="relative size-2 rounded-full bg-sky-400" />
              </span>
            ) : (
              <span className="flex size-4 items-center justify-center">
                <span className="size-2 rounded-full border border-neutral-700" />
              </span>
            )}
            <span className={cn("flex-1", state === "todo" ? "text-neutral-600" : state === "run" ? "text-sky-300" : "text-neutral-300")}>
              {label}
            </span>
            <span className="text-neutral-600">{dur}</span>
          </li>
        ))}
      </ul>
      <div className="border-t border-white/[0.07] bg-black/40 px-4 py-3.5 font-mono text-[11px] leading-relaxed text-neutral-500">
        <p className="text-neutral-400">$ kubectl set image deploy/payments-api web=…:v2.14.0</p>
        <p>deployment.apps/payments-api image updated</p>
        <p>waiting for rollout: 2/3 replicas updated…</p>
      </div>
    </div>
  );
}

/* ── prod gate ───────────────────────────────────────────────────────── */

function ProdGate() {
  return (
    <section className="border-y border-white/[0.07] bg-[#0b0b0c]">
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6">
        <SectionHead
          center
          eyebrow="The production gate"
          title="Harder to enter than to leave."
          lede="Production deserves asymmetry: deliberate on the way in, instant on the way out."
        />
        <div className="mx-auto mt-12 grid max-w-4xl grid-cols-1 gap-5 sm:grid-cols-2">
          <Reveal delay={0.05}>
            <div className="h-full rounded-xl border border-amber-500/20 bg-[#090909] p-6">
              <span className="flex size-9 items-center justify-center rounded-lg border border-amber-500/25 bg-amber-500/10">
                <Lock aria-hidden className="size-4 text-amber-400" />
              </span>
              <h3 className="mt-4 font-semibold text-neutral-100">
                Entering prod asks for the password
              </h3>
              <p className="mt-2 text-sm leading-relaxed text-neutral-500">
                With <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono text-xs text-neutral-300">RP_PROD_PASSWORD</code>{" "}
                set, anything that lands in the last ring — a promotion, a direct
                seed, even enabling auto-promote into it — must carry the
                production password.
              </p>
            </div>
          </Reveal>
          <Reveal delay={0.12}>
            <div className="h-full rounded-xl border border-emerald-500/20 bg-[#090909] p-6">
              <span className="flex size-9 items-center justify-center rounded-lg border border-emerald-500/25 bg-emerald-500/10">
                <RotateCcw aria-hidden className="size-4 text-emerald-400" />
              </span>
              <h3 className="mt-4 font-semibold text-neutral-100">
                Leaving prod never waits
              </h3>
              <p className="mt-2 text-sm leading-relaxed text-neutral-500">
                Rollbacks are deliberately exempt from the gate. When you are
                paged at 3am, incident response is never blocked by a password
                prompt.
              </p>
            </div>
          </Reveal>
        </div>
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
        <div className="mt-12 grid grid-cols-1 gap-5 lg:grid-cols-2">
          <Reveal delay={0.05}>
            <div className="h-full rounded-xl border border-white/[0.07] bg-[#0b0b0c] p-6">
              <div className="flex items-center gap-3">
                <span className="flex size-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04]">
                  <Container aria-hidden className="size-4 text-neutral-300" />
                </span>
                <h3 className="font-semibold text-neutral-100">Kubernetes apps</h3>
              </div>
              <p className="mt-4 text-sm leading-relaxed text-neutral-500">
                The kubectl deployer runs{" "}
                <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono text-xs text-neutral-300">set image</code>{" "}
                +{" "}
                <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono text-xs text-neutral-300">rollout status</code>,
                authenticated in-cluster via the pod&rsquo;s ServiceAccount.
                Battle-tested rollout semantics with a tiny dependency tree.
              </p>
            </div>
          </Reveal>
          <Reveal delay={0.12}>
            <div className="h-full rounded-xl border border-white/[0.07] bg-[#0b0b0c] p-6">
              <div className="flex items-center gap-3">
                <span className="flex size-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04]">
                  <Workflow aria-hidden className="size-4 text-neutral-300" />
                </span>
                <h3 className="font-semibold text-neutral-100">VM apps with existing CI</h3>
              </div>
              <p className="mt-4 text-sm leading-relaxed text-neutral-500">
                The GitHub Actions deployer dispatches the workflow you already
                have and waits for the run to conclude — same health checks, same
                auto-rollback. Seeded versions are validated against the
                repo&rsquo;s real branches and tags, so a typo&rsquo;d ref never
                launches a doomed run.
              </p>
            </div>
          </Reveal>
        </div>
      </div>
    </section>
  );
}

/* ── config ──────────────────────────────────────────────────────────── */

function Config() {
  return (
    <section id="config" className="scroll-mt-20 border-y border-white/[0.07] bg-[#0b0b0c]">
      <div className="mx-auto grid max-w-6xl grid-cols-1 items-center gap-12 px-4 py-24 sm:px-6 lg:grid-cols-2">
        <div>
          <SectionHead
            eyebrow="Onboarding"
            title="An app is a block of YAML."
            lede="No plugin, no SDK, no rebuild. Declare where each ring lives and how to check its health, apply the config, and the app appears in the console. Apps only define the rings they actually live in."
          />
        </div>
        <Reveal delay={0.1}>
          <div className="overflow-hidden rounded-xl border border-white/10 bg-[#090909]">
            <div className="border-b border-white/[0.07] px-4 py-2 font-mono text-[11px] text-neutral-500">
              config.yaml
            </div>
            <pre className="overflow-x-auto p-4 font-mono text-xs leading-relaxed">
              <code>
                <Y k="apps" />{"\n"}
                {"  - "}<Y k="name" v="billing-worker" />{"\n"}
                {"    "}<Y k="rings" />{"\n"}
                {"      "}<Y k="int" />{"\n"}
                {"        "}<Y k="namespace" v="int" />{"\n"}
                {"        "}<Y k="deployment" v="billing-worker" />{"\n"}
                {"        "}<Y k="container" v="worker" />{"\n"}
                {"        "}<Y k="image" v="registry.example.com/billing-worker" />{"\n"}
                {"        "}<Y k="health_url" v="http://billing-worker.int.svc/health" />{"\n"}
                {"      "}<Y k="test" />{"\n"}
                {"        "}<C t="# …same shape, per ring" />{"\n"}
                {"      "}<Y k="prod" />{"\n"}
                {"        "}<C t="# only the rings the app lives in" />
              </code>
            </pre>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

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
              <div className="h-full rounded-xl border border-white/[0.07] bg-[#0b0b0c] p-5">
                <div className="flex items-center justify-between">
                  <span className="flex size-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04]">
                    <Icon aria-hidden className="size-4 text-neutral-300" />
                  </span>
                  <span
                    className={cn(
                      "rounded-full border px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider",
                      stage === "planned"
                        ? "border-emerald-500/25 text-emerald-400/90"
                        : "border-white/15 text-neutral-500",
                    )}
                  >
                    {stage}
                  </span>
                </div>
                <h3 className="mt-4 text-sm font-semibold text-neutral-100">{title}</h3>
                <p className="mt-2 text-[13px] leading-relaxed text-neutral-500">{body}</p>
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
          <div className="mt-10 divide-y divide-white/[0.07] rounded-xl border border-white/[0.07] bg-[#0b0b0c]">
            {qas.map(([q, a]) => (
              <details key={q} className="group px-5 py-4 open:pb-5">
                <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-sm font-medium text-neutral-200 [&::-webkit-details-marker]:hidden">
                  {q}
                  <span
                    aria-hidden
                    className="text-neutral-600 transition-transform duration-200 group-open:rotate-45"
                  >
                    +
                  </span>
                </summary>
                <p className="mt-3 text-sm leading-relaxed text-neutral-500">{a}</p>
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
    <section className="relative overflow-hidden border-t border-white/[0.07]">
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
          <h2 className="text-balance text-3xl font-semibold tracking-tight text-neutral-50 sm:text-4xl">
            Bring release discipline to your cluster.
          </h2>
          <p className="mx-auto mt-4 max-w-xl text-pretty leading-relaxed text-neutral-400">
            Try it in one command — the defaults use an in-memory store and a
            no-op deployer, so there is nothing to install and nothing to break.
          </p>
          <div className="mx-auto mt-8 max-w-xl overflow-hidden rounded-xl border border-white/10 bg-[#0b0b0c] text-left">
            <pre className="overflow-x-auto px-4 py-3.5 font-mono text-xs leading-relaxed">
              <code>
                <span className="text-neutral-600">$ </span>
                <span className="text-neutral-100">go run ./cmd/ringpromoter --config config.yaml</span>
                {"\n"}
                <span className="text-neutral-600"># → http://localhost:8080 · token: local-dev-token</span>
              </code>
            </pre>
          </div>
          <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
            <Link
              href="/"
              className="rounded-md bg-neutral-100 px-5 py-2.5 text-sm font-medium text-neutral-900 transition-colors hover:bg-white"
            >
              Open the console
            </Link>
            <a
              href={GITHUB}
              target="_blank"
              rel="noreferrer"
              className="rounded-md border border-white/15 px-5 py-2.5 text-sm font-medium text-neutral-200 transition-colors hover:bg-white/[0.06]"
            >
              View on GitHub
            </a>
          </div>
        </Reveal>
      </div>
    </section>
  );
}
