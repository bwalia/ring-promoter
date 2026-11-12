import type { Metadata } from "next";
import {
  Bullets,
  Callout,
  FactTable,
  In,
  LegalPage,
  MailLink,
  Out,
  Section,
} from "@/components/legal/legal-page";
import { APP_STORE_URL } from "@/components/app-store-badge";
import { APP_NAME, CONTACT_EMAIL, GITHUB_URL, PUBLISHER } from "@/lib/legal";

export const metadata: Metadata = {
  title: "Support — Ring Promoter",
  description:
    "Help with the Ring Promoter iOS app: requirements, connecting to a control plane, trying it without a server, and fixes for the common problems.",
};

const ISSUES = `${GITHUB_URL}/issues`;

export default function SupportPage() {
  return (
    <LegalPage
      title="Support"
      intro={`Help with the ${APP_NAME} app for iOS. If what you need is not here, write to us — a real person reads it.`}
    >
      <Section id="contact" heading="Get in touch">
        <Callout>
          Email <MailLink email={CONTACT_EMAIL} />. Please say which iOS version
          and device you are on, and what the app showed you. A screenshot is
          worth a paragraph.
        </Callout>
        <p>
          Bugs and feature requests can also go to{" "}
          <Out href={ISSUES}>GitHub issues</Out>, where they are public and you
          can follow the fix.
        </p>
      </Section>

      <Section id="download" heading="Get the app">
        <p>
          {APP_NAME} is free on the{" "}
          <Out href={APP_STORE_URL}>App Store</Out> for iPhone and iPad.
        </p>
      </Section>

      <Section id="requirements" heading="What you need">
        <FactTable
          caption="Requirements"
          head={["Requirement", "Detail"]}
          rows={[
            ["iOS", "17.0 or later"],
            ["Device", "iPhone or iPad"],
            [
              "A control plane",
              <>
                A Ring Promoter server of your own, reachable from the device,
                plus its API token. The app cannot mint a token and we do not
                host a server for you — ask whoever runs your control plane.
              </>,
            ],
            [
              "Nothing else",
              "No account, no sign-up and no subscription. The app has no backend of ours.",
            ],
          ]}
        />
      </Section>

      <Section id="demo" heading="Trying it without a server">
        <p>
          Tap <strong className="font-medium text-neutral-100">Explore in demo mode</strong>{" "}
          on the first screen, or{" "}
          <strong className="font-medium text-neutral-100">
            Settings → Switch to demo mode
          </strong>{" "}
          later. Demo mode runs the whole app from data bundled inside it, with
          no server and no network — including seeding, promoting, rolling back,
          live job progress and the gates that refuse an illegal promotion.
        </p>
        <FactTable
          caption="Demo mode credentials"
          head={["Prompt", "Value"]}
          rows={[
            [
              "Production password",
              // Wrapped in a fragment because a bare keyable element inside an
              // array literal trips react/jsx-key; FactTable keys the rows.
              <>
                <code className="font-mono text-[13px] text-neutral-300">demo</code>
              </>,
            ],
            [
              "Change-request code",
              <>
                <code className="font-mono text-[13px] text-neutral-300">test</code>
              </>,
            ],
          ]}
        />
        <p>
          To see a failure handled, seed any version string containing{" "}
          <code className="font-mono text-[13px] text-neutral-300">bad</code> —
          the health check fails and the ring rolls itself back.
        </p>
      </Section>

      <Section id="connecting" heading="Connecting to your control plane">
        <p>
          Tap the instance banner at the top of any screen, then{" "}
          <strong className="font-medium text-neutral-100">Add a cluster</strong>{" "}
          (or <strong className="font-medium text-neutral-100">Settings → Manage clusters → +</strong>
          ). Two fields matter:
        </p>
        <Bullets
          items={[
            <>
              <strong className="font-medium text-neutral-100">URL</strong> —
              for example{" "}
              <code className="font-mono text-[13px] text-neutral-300">
                https://ring-promoter.example.com
              </code>
              . A bare hostname gets{" "}
              <code className="font-mono text-[13px] text-neutral-300">https://</code>{" "}
              added and a trailing slash is trimmed.
            </>,
            <>
              <strong className="font-medium text-neutral-100">Token</strong> —
              whatever the server was started with (its{" "}
              <code className="font-mono text-[13px] text-neutral-300">
                RP_API_TOKEN
              </code>
              ). It is written to the Keychain only after the server accepts it.
            </>,
          ]}
        />
        <p>
          The app validates in two steps so you can tell which thing is wrong:
          it asks the URL whether a Ring Promoter is there at all, then asks
          whether your token works. An unreachable host, a TLS failure, a server
          that is not Ring Promoter and a rejected token are four different
          messages.
        </p>
      </Section>

      <Section id="troubleshooting" heading="Common problems">
        <FactTable
          caption="Troubleshooting"
          head={["What you see", "What to do"]}
          rows={[
            [
              "“Cannot reach the server”",
              <>
                The device cannot route to the host. If the control plane is on
                a private network or behind a VPN, the device needs to be on
                that network too. Check the URL and port, and try the same URL
                in Safari on the device.
              </>,
            ],
            [
              "A certificate or TLS error",
              <>
                The server&apos;s certificate is not trusted by iOS — expired,
                self-signed or missing an intermediate. Fix it on the server.
                The app has no option to skip certificate validation, and will
                not grow one: it can deploy to production, which is the wrong
                place for that switch.
              </>,
            ],
            [
              "“Sign in again”, or a rejected token when adding a cluster",
              <>
                The server is reachable but the token is wrong, and the app
                sends you back to the token field rather than showing a
                dismissable error. Tokens are compared exactly — watch for a
                trailing space or newline when pasting. Get a fresh one from
                whoever runs the control plane.
              </>,
            ],
            [
              "A warning triangle on an instance",
              <>
                It is an{" "}
                <code className="font-mono text-[13px] text-neutral-300">
                  http://
                </code>{" "}
                address, so your token crosses the network in clear text. Fine
                for a local development server, not for anything else.
              </>,
            ],
            [
              "The widget is blank",
              <>
                It draws the snapshot the app writes after a successful refresh,
                so open the app once and let the Overview load. The widget never
                makes network calls of its own and holds no credential.
              </>,
            ],
            [
              "Face ID will not let me in",
              <>
                Both biometric settings fall back to the device passcode, so a
                failed scan never locks you out of a rollback. If you have no
                passcode set at all, the typed-name confirmation remains as the
                guard.
              </>,
            ],
            [
              "A link to an old job shows “not found”",
              <>
                The control plane keeps jobs in memory and caps the history, so
                a deep link to an old job can expire. The promotion itself is
                still recorded in History.
              </>,
            ],
            [
              "I allowed notifications but never get any",
              <>
                Expected for now. Permission and device registration work, but
                the control plane has no push sender yet, so nothing is sent.
                Settings says so on the same screen.
              </>,
            ],
            [
              "An action is greyed out",
              <>
                The app only offers legal moves. A promotion needs a healthy
                source ring and goes one ring at a time; some rings additionally
                require a QA sign-off, an open maintenance window or a
                change-request code. The sheet says which one is missing.
              </>,
            ],
          ]}
        />
      </Section>

      <Section id="data" heading="Your data and your token">
        <p>
          Your token lives in the iOS Keychain and is excluded from backups and
          device transfers; your server addresses live in ordinary app
          preferences. Nothing is sent to us.{" "}
          <strong className="font-medium text-neutral-100">
            Settings → Forget every cluster
          </strong>{" "}
          erases every saved record, every token and the widget snapshot in one
          action — do that before handing a device on. Deleting the app removes
          all of it too.
        </p>
        <p>
          The full detail is in the <In href="/privacy">Privacy Policy</In>.
        </p>
      </Section>

      <Section id="more" heading="More">
        <Bullets
          items={[
            <>
              <In href="/">What Ring Promoter does</In> — the control
              plane behind the app.
            </>,
            <>
              <Out href={GITHUB_URL}>Source and documentation on GitHub</Out>.
            </>,
            <>
              <In href="/terms">Terms of Use</In> and{" "}
              <In href="/privacy">Privacy Policy</In>.
            </>,
          ]}
        />
        <p className="text-[14px] text-neutral-500">
          {APP_NAME} is published by {PUBLISHER}.
        </p>
      </Section>
    </LegalPage>
  );
}
