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
import {
  APP_NAME,
  BUNDLE_ID,
  CONTACT_EMAIL,
  GITHUB_URL,
  JURISDICTION,
  PUBLISHER,
  SITE_URL,
} from "@/lib/legal";

export const metadata: Metadata = {
  title: "Privacy Policy — Ring Promoter",
  description:
    "What the Ring Promoter iOS app stores, what leaves your device, and what we receive. The app collects no personal data.",
};

export default function PrivacyPage() {
  return (
    <LegalPage
      title="Privacy Policy"
      intro={`This policy covers the ${APP_NAME} app for iOS (bundle identifier ${BUNDLE_ID}) and this website. The short version: the app collects nothing about you, sends nothing to us, and talks only to the server whose address you type in yourself.`}
    >
      <Section id="summary" heading="The short version">
        <Callout>
          {APP_NAME} is a client for a control plane that <em>you</em> run. It
          has no accounts, no sign-up and no backend of ours. We operate no
          servers that the app contacts, we receive no analytics, no crash
          reports and no telemetry from it, and it contains no third-party SDK
          that could add any. In App Store Connect the app is declared as{" "}
          <strong className="font-medium text-neutral-100">
            Data Not Collected
          </strong>
          , and that declaration is accurate.
        </Callout>
        <p>
          Because the app has no server of ours behind it, most of what a
          privacy policy usually describes does not exist here. The rest of
          this page says precisely what is stored, where it is stored, and who
          — if anyone — can see it.
        </p>
      </Section>

      <Section id="who" heading="Who is responsible">
        <p>
          The app and this website are published by {PUBLISHER}. For anything
          in this policy, write to <MailLink email={CONTACT_EMAIL} />.
        </p>
        <p>
          Where data protection law applies, {PUBLISHER} is the controller for
          this website only. For everything inside the app, you are the
          controller of your own data: it stays on your device or goes to
          infrastructure you operate.
        </p>
      </Section>

      <Section id="stored" heading="What the app stores on your device">
        <p>
          Everything in this table stays on the device. None of it is
          transmitted to us, and there is no mechanism in the app by which it
          could be.
        </p>
        <FactTable
          caption="What the app stores and where"
          head={["What", "Where it is kept"]}
          rows={[
            [
              "Control plane API token",
              <>
                The iOS Keychain, with accessibility{" "}
                <code className="font-mono text-[13px] text-neutral-300">
                  WhenUnlockedThisDeviceOnly
                </code>
                . It is unreadable while the device is locked, and is excluded
                from encrypted backups and from device-to-device transfer. You
                re-enter it after restoring a device — deliberately, so a
                control-plane token never travels inside a backup.
              </>,
            ],
            [
              "Server address, display name and colour",
              <>
                Standard app preferences (<code className="font-mono text-[13px] text-neutral-300">UserDefaults</code>), kept
                out of the Keychain so the boundary between secret and
                not-secret is obvious. Covered by your device backup.
              </>,
            ],
            [
              "Production password",
              <>
                Nowhere. It is held in memory for the single request it
                authorises and then discarded. It is never written to disk, to
                the Keychain or to a log.
              </>,
            ],
            [
              "Home Screen widget snapshot",
              <>
                A file in the app&apos;s App Group container: application
                names, version strings, health flags and the instance&apos;s
                display name. It contains no token, no password and no server
                address — a unit test asserts that.
              </>,
            ],
            [
              "Notification device token",
              <>
                Memory only, for as long as the app is running. There is
                nowhere to send it: the control plane has no push sender yet,
                so nothing ever posts it anywhere.
              </>,
            ],
            [
              "Display and security settings",
              "Standard app preferences — refresh interval, appearance, and whether Face ID guards the app and production actions.",
            ],
          ]}
        />
      </Section>

      <Section id="leaves" heading="What leaves your device">
        <p>
          Only requests to the control plane address you entered. Those
          requests carry three headers and nothing else of note:{" "}
          <code className="font-mono text-[13px] text-neutral-300">Accept</code>
          ,{" "}
          <code className="font-mono text-[13px] text-neutral-300">
            Content-Type
          </code>{" "}
          and{" "}
          <code className="font-mono text-[13px] text-neutral-300">
            Authorization
          </code>{" "}
          carrying your bearer token. The app adds no device identifier, no
          advertising identifier, no installation id and no user agent of its
          own.
        </p>
        <Bullets
          items={[
            <>
              <strong className="font-medium text-neutral-100">
                That server is yours, not ours.
              </strong>{" "}
              What it logs about a promotion — who promoted what, and when — is
              governed by your own policies, not this one. We have no access to
              it.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                No analytics or crash reporting.
              </strong>{" "}
              The app has no analytics SDK, no crash reporter and no logging of
              request bodies, headers or responses. Your token and production
              password appear in no log and no crash report.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Demo mode is entirely offline.
              </strong>{" "}
              It runs from data bundled inside the app and opens no network
              connection at all.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Apple may give us aggregate statistics.
              </strong>{" "}
              If you have opted in to sharing analytics with developers, Apple
              may show us anonymous, aggregated download, crash and usage
              counts. We never receive anything that identifies you. This is
              controlled by Apple, in Settings → Privacy &amp; Security →
              Analytics &amp; Improvements.
            </>,
          ]}
        />
      </Section>

      <Section id="transport" heading="How it is protected in transit">
        <p>
          The app sets no App Transport Security exceptions, so every{" "}
          <code className="font-mono text-[13px] text-neutral-300">https://</code>{" "}
          connection gets the iOS default: TLS 1.2 or better, forward secrecy
          and full certificate validation. There is no way to disable
          certificate checking.
        </p>
        <p>
          A plain{" "}
          <code className="font-mono text-[13px] text-neutral-300">http://</code>{" "}
          address is accepted, because a local development control plane is a
          legitimate target — but the app warns you when you add one, and marks
          it in the instance list, because your token crosses the network in
          clear text.
        </p>
      </Section>

      <Section id="biometrics" heading="Face ID and Touch ID">
        <p>
          You can require biometric authentication to open the app and to
          confirm anything that deploys to production. Authentication is
          performed by iOS: your face and fingerprint data never leave the
          Secure Enclave and are never available to the app, which receives
          only a yes or no. Nothing biometric is stored or transmitted by the
          app.
        </p>
      </Section>

      <Section id="notifications" heading="Notifications">
        <p>
          You may grant notification permission in Settings. Doing so registers
          the device with Apple&apos;s push service and the app receives a
          device token, which it holds in memory only. No notification can
          currently be delivered, because the control plane has no sender — the
          app says so plainly in Settings rather than implying otherwise. If
          that changes in a future release, this policy will be updated before
          any token is transmitted anywhere.
        </p>
      </Section>

      <Section id="children" heading="Children">
        <p>
          {APP_NAME} is a tool for software operations engineers. It is not
          directed at children, has no content designed for them, and we do not
          knowingly collect information from anyone — of any age.
        </p>
      </Section>

      <Section id="deletion" heading="Deleting your data">
        <p>
          Because nothing reaches us, there is nothing for us to delete and no
          request to make. You control all of it from the device:
        </p>
        <Bullets
          items={[
            "Deleting a saved control plane also deletes its token from the Keychain.",
            "Settings → Forget every cluster erases every saved record, every token and the widget snapshot in one action — intended for handing a device on.",
            "Deleting the app removes its preferences, its Keychain items and its App Group container with it.",
          ]}
        />
        <p>
          Anything recorded by your own control plane is deleted through your
          own systems. We cannot reach it.
        </p>
      </Section>

      <Section id="rights" heading="Your rights">
        <p>
          Where the UK GDPR, EU GDPR or a comparable law applies, you have
          rights of access, rectification, erasure, restriction, objection and
          portability over personal data a controller holds about you. In
          respect of the app, {PUBLISHER} holds none, so there is nothing to
          access or erase. In respect of this website, see the next section, and
          write to <MailLink email={CONTACT_EMAIL} /> to exercise any right.
          You may also complain to your local supervisory authority.
        </p>
      </Section>

      <Section id="website" heading="This website">
        <p>
          Separately from the app, <Out href={SITE_URL}>{SITE_URL.replace("https://", "")}</Out>{" "}
          uses Google Analytics to count visits and understand which pages are
          read. Google sets cookies and processes your IP address and
          approximate location on our behalf for that purpose; the legal basis
          is our legitimate interest in understanding how the site is used. No
          data from the app feeds into this, and the two are not linked.
        </p>
        <p>
          You can prevent it entirely with Google&apos;s{" "}
          <Out href="https://tools.google.com/dlpage/gaoptout">
            opt-out browser add-on
          </Out>
          , a tracker-blocking extension, or by not visiting the site. The pages
          are also readable in the project&apos;s{" "}
          <Out href={GITHUB_URL}>GitHub repository</Out>, which carries no
          analytics of ours.
        </p>
      </Section>

      <Section id="processors" heading="Who else is involved">
        <Bullets
          items={[
            <>
              <strong className="font-medium text-neutral-100">Apple</strong> —
              distributes the app and processes your purchase, download and any
              diagnostics you choose to share, under{" "}
              <Out href="https://www.apple.com/legal/privacy/">
                Apple&apos;s own privacy policy
              </Out>
              .
            </>,
            <>
              <strong className="font-medium text-neutral-100">Google</strong> —
              analytics for this website only, as described above.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Your own infrastructure
              </strong>{" "}
              — the control plane you point the app at, and whoever hosts it.
              That relationship is yours.
            </>,
          ]}
        />
        <p>
          We sell no data, share none for advertising, and run no profiling or
          automated decision-making. The app does not track you across apps or
          websites and requests no permission to do so.
        </p>
      </Section>

      <Section id="changes" heading="Changes to this policy">
        <p>
          If this policy changes, the date at the top of the page changes with
          it, and a change that affects what the app does will ship alongside
          the app release that introduces it. The history of this page is public
          in the <Out href={GITHUB_URL}>repository</Out>.
        </p>
      </Section>

      <Section id="contact" heading="Contact">
        <p>
          {PUBLISHER} — <MailLink email={CONTACT_EMAIL} />. This policy is
          governed by the law of {JURISDICTION}. See also our{" "}
          <In href="/terms">Terms of Use</In> and the{" "}
          <In href="/support">support page</In>.
        </p>
      </Section>
    </LegalPage>
  );
}
