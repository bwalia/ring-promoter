import type { ReactNode } from "react";
import Link from "next/link";
import { APP_NAME, GITHUB_URL, LAST_UPDATED } from "@/lib/legal";

/**
 * Shared chrome for /privacy, /terms and /support.
 *
 * Same dark control-room palette as the landing page, with explicit colours
 * rather than theme tokens — these pages are linked from the App Store and
 * from inside the iOS app, so they must render identically for everyone and
 * must not depend on the console's theme toggle.
 *
 * Styled with plain utilities instead of `prose`: @tailwindcss/typography is
 * not a dependency of this project and three legal pages are not a reason to
 * add one.
 */
export function LegalPage({
  title,
  intro,
  children,
}: {
  title: string;
  intro: string;
  children: ReactNode;
}) {
  return (
    <div className="min-h-screen bg-[#090909] text-neutral-300 antialiased selection:bg-emerald-500/25 selection:text-white">
      <LegalNav />
      <main className="mx-auto max-w-3xl px-4 pt-12 pb-20 sm:px-6">
        <h1 className="font-display text-3xl font-bold tracking-tight text-neutral-50 sm:text-4xl">
          {title}
        </h1>
        <p className="mt-4 text-[15px] leading-relaxed text-neutral-400">
          {intro}
        </p>
        <p className="mt-6 font-mono text-xs text-neutral-500">
          Last updated {LAST_UPDATED}
        </p>
        <hr className="mt-8 border-white/[0.07]" />
        <div className="mt-10 space-y-10">{children}</div>
      </main>
      <LegalFooter />
    </div>
  );
}

/** One numbered-feeling section. Headings carry ids so clauses are linkable. */
export function Section({
  id,
  heading,
  children,
}: {
  id: string;
  heading: string;
  children: ReactNode;
}) {
  return (
    <section id={id} className="scroll-mt-20">
      <h2 className="font-display text-xl font-semibold tracking-tight text-neutral-100">
        {heading}
      </h2>
      <div className="mt-4 space-y-4 text-[15px] leading-relaxed text-neutral-300">
        {children}
      </div>
    </section>
  );
}

export function Bullets({ items }: { items: ReactNode[] }) {
  return (
    <ul className="space-y-2.5 pl-1">
      {items.map((item, i) => (
        <li key={i} className="flex gap-3">
          <span aria-hidden className="mt-2 size-1.5 shrink-0 rounded-full bg-emerald-500/70" />
          <span className="min-w-0">{item}</span>
        </li>
      ))}
    </ul>
  );
}

/** A quiet emphasis box for the one or two things a reader must not miss. */
export function Callout({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/[0.04] px-4 py-3.5 text-[14.5px] leading-relaxed text-neutral-200">
      {children}
    </div>
  );
}

/** Two-column facts — what is stored where, who is responsible for what. */
export function FactTable({
  caption,
  head,
  rows,
}: {
  caption?: string;
  head: [string, string];
  rows: [ReactNode, ReactNode][];
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-white/[0.07]">
      <table className="w-full border-collapse text-left text-[14px]">
        {caption ? <caption className="sr-only">{caption}</caption> : null}
        <thead>
          <tr className="border-b border-white/[0.07] bg-white/[0.02]">
            <th scope="col" className="px-4 py-2.5 font-medium text-neutral-200">
              {head[0]}
            </th>
            <th scope="col" className="px-4 py-2.5 font-medium text-neutral-200">
              {head[1]}
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} className="border-b border-white/[0.05] last:border-0 align-top">
              <th scope="row" className="px-4 py-3 font-normal text-neutral-200">
                {row[0]}
              </th>
              <td className="px-4 py-3 text-neutral-400">{row[1]}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function MailLink({ email }: { email: string }) {
  return (
    <a
      href={`mailto:${email}`}
      className="text-emerald-400 underline decoration-emerald-500/40 underline-offset-2 transition-colors hover:text-emerald-300"
    >
      {email}
    </a>
  );
}

export function Out({ href, children }: { href: string; children: ReactNode }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="text-emerald-400 underline decoration-emerald-500/40 underline-offset-2 transition-colors hover:text-emerald-300"
    >
      {children}
    </a>
  );
}

export function In({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      className="text-emerald-400 underline decoration-emerald-500/40 underline-offset-2 transition-colors hover:text-emerald-300"
    >
      {children}
    </Link>
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

function LegalNav() {
  return (
    <header className="sticky top-0 z-40 border-b border-white/[0.07] bg-[#090909]/85 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-3xl items-center justify-between px-4 sm:px-6">
        <Link href="/landing" className="flex items-center gap-2.5">
          <RingMark className="size-6" />
          <span className="font-display text-[15px] font-semibold tracking-tight text-neutral-50">
            {APP_NAME}
          </span>
        </Link>
        <nav aria-label="Legal" className="flex items-center gap-5 text-[13px] text-neutral-400">
          <Link href="/privacy" className="transition-colors hover:text-neutral-100">
            Privacy
          </Link>
          <Link href="/terms" className="transition-colors hover:text-neutral-100">
            Terms
          </Link>
          <Link href="/support" className="transition-colors hover:text-neutral-100">
            Support
          </Link>
        </nav>
      </div>
    </header>
  );
}

function LegalFooter() {
  return (
    <footer className="border-t border-white/[0.07]">
      <div className="mx-auto flex max-w-3xl flex-col gap-3 px-4 py-8 text-[13px] text-neutral-500 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <span>{APP_NAME} — a control plane for release promotion</span>
        <div className="flex items-center gap-5">
          <Link href="/landing" className="transition-colors hover:text-neutral-200">
            Overview
          </Link>
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noreferrer"
            className="transition-colors hover:text-neutral-200"
          >
            GitHub ↗
          </a>
        </div>
      </div>
    </footer>
  );
}
