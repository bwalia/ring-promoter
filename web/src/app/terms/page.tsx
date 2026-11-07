import type { Metadata } from "next";
import {
  Bullets,
  Callout,
  In,
  LegalPage,
  MailLink,
  Out,
  Section,
} from "@/components/legal/legal-page";
import {
  APP_NAME,
  CONTACT_EMAIL,
  GITHUB_URL,
  JURISDICTION,
  POSTAL_ADDRESS,
  PUBLISHER,
} from "@/lib/legal";

export const metadata: Metadata = {
  title: "Terms of Use — Ring Promoter",
  description:
    "Licence terms for the Ring Promoter iOS app, including the minimum terms Apple requires in a developer-supplied end-user licence agreement.",
};

export default function TermsPage() {
  return (
    <LegalPage
      title="Terms of Use"
      intro={`These terms are the end-user licence agreement for the ${APP_NAME} app for iOS. By downloading or using the app you accept them. If you do not accept them, do not use the app.`}
    >
      <Section id="what" heading="1. What this app is">
        <p>
          {APP_NAME} is a client. It displays and operates a Ring Promoter
          control plane that <strong className="font-medium text-neutral-100">you</strong>{" "}
          run on your own infrastructure. We host nothing on your behalf, hold
          no account for you, and have no access to your systems or your data.
          Without a control plane of your own the app is useful only in its
          offline demo mode.
        </p>
      </Section>

      <Section id="deployments" heading="2. The app performs real deployments">
        <Callout>
          Used against a live control plane, this app changes production
          systems. It seeds, promotes and rolls back real application versions
          in real environments. You are solely responsible for every action you
          take with it and for its consequences, including downtime, data loss
          and failed releases.
        </Callout>
        <p>
          The app provides guards — health gating, a one-hop-at-a-time
          promotion rule, a production password, typed confirmation and
          optional biometric authentication — and those guards are enforced by
          your control plane, not by us. They reduce the chance of a mistake.
          They do not remove it, and they are not a substitute for your own
          change management, testing and backups.
        </p>
        <p>
          Nothing in the app should be relied on as the sole safeguard before a
          production change.
        </p>
      </Section>

      <Section id="credentials" heading="3. Your credentials and your server">
        <Bullets
          items={[
            "You are responsible for the API token you enter, for keeping it secret, and for revoking it if a device is lost. The app stores it in the iOS Keychain and provides a way to erase it, but it cannot revoke it for you.",
            "You are responsible for securing your control plane, including serving it over HTTPS with a valid certificate. The app warns you when you add a plain http:// address because your token then crosses the network in clear text.",
            "You must be authorised to operate the systems you point the app at.",
          ]}
        />
      </Section>

      <Section id="licence" heading="4. Licence">
        <p>
          {PUBLISHER} grants you a personal, non-exclusive, non-transferable,
          revocable licence to use the app on Apple-branded devices you own or
          control, as permitted by the Usage Rules in the Apple Media Services
          Terms and Conditions. You may not copy, redistribute, rent, sell,
          sublicence, reverse-engineer or modify the app except where that
          restriction is prohibited by law. All rights not granted here are
          reserved.
        </p>
      </Section>

      <Section id="warranty" heading="5. No warranty">
        <p>
          The app is provided &ldquo;as is&rdquo; and &ldquo;as
          available&rdquo;, without warranty of any kind, whether express or
          implied, including any implied warranty of merchantability, fitness
          for a particular purpose, accuracy or non-infringement. We do not
          warrant that the app will be uninterrupted, error-free, compatible
          with any particular control plane version, or that any defect will be
          corrected.
        </p>
        <p>
          Nothing in these terms excludes or limits any right you have as a
          consumer that cannot be excluded or limited by law.
        </p>
      </Section>

      <Section id="liability" heading="6. Limitation of liability">
        <p>
          To the fullest extent permitted by law, {PUBLISHER} is not liable for
          any indirect, incidental, special, consequential or punitive loss, nor
          for any loss of profit, revenue, data, goodwill or anticipated
          savings, arising out of or in connection with your use of the app —
          including any deployment, promotion, rollback or outage, whether
          caused by the app, by your control plane, or by an action you took
          using the app.
        </p>
        <p>
          Nothing here limits liability for death or personal injury caused by
          negligence, for fraud or fraudulent misrepresentation, or for any
          other liability that cannot lawfully be limited.
        </p>
      </Section>

      <Section id="price" heading="7. Price and purchases">
        <p>
          The app contains no in-app purchases, no subscriptions and no
          advertising. Any price is set in the App Store, and purchases,
          refunds and billing are handled by Apple under its own terms, not by
          us.
        </p>
      </Section>

      <Section id="changes" heading="8. Changes and availability">
        <p>
          We may update the app, change its features, or stop publishing it.
          These terms may change; the date at the top of the page changes with
          them, and continuing to use the app after a change means you accept
          the revised terms. The history of this page is public in the{" "}
          <Out href={GITHUB_URL}>repository</Out>.
        </p>
      </Section>

      <Section id="termination" heading="9. Termination">
        <p>
          This licence ends if you stop complying with these terms, and you may
          end it at any time by deleting the app. On termination you must stop
          using the app; the sections on liability, warranty and governing law
          survive.
        </p>
      </Section>

      <Section id="apple" heading="10. Terms required by Apple">
        <p>
          Apple requires the following to form part of any licence agreement a
          developer supplies with an App Store app. Where anything above
          conflicts with this section, this section prevails.
        </p>
        <Bullets
          items={[
            <>
              <strong className="font-medium text-neutral-100">
                Acknowledgement.
              </strong>{" "}
              This agreement is between you and {PUBLISHER} only, and not with
              Apple. {PUBLISHER}, not Apple, is solely responsible for the app
              and its content. This agreement does not provide for usage rules
              less restrictive than the Usage Rules in the Apple Media Services
              Terms and Conditions.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Scope of licence.
              </strong>{" "}
              The licence granted to you is limited to a non-transferable
              licence to use the app on any Apple-branded products that you own
              or control, as permitted by those Usage Rules.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Maintenance and support.
              </strong>{" "}
              {PUBLISHER} is solely responsible for providing any maintenance
              and support for the app, as specified in this agreement or as
              required by applicable law. Apple has no obligation whatsoever to
              furnish any maintenance or support for the app.
            </>,
            <>
              <strong className="font-medium text-neutral-100">Warranty.</strong>{" "}
              {PUBLISHER} is solely responsible for any product warranties,
              whether express or implied by law, to the extent not effectively
              disclaimed. If the app fails to conform to any applicable
              warranty, you may notify Apple, and Apple will refund the
              purchase price of the app to you; to the maximum extent permitted
              by applicable law, Apple will have no other warranty obligation
              whatsoever with respect to the app.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Product claims.
              </strong>{" "}
              {PUBLISHER}, not Apple, is responsible for addressing any claims
              by you or any third party relating to the app or your possession
              and use of it, including product liability claims, any claim that
              the app fails to conform to any applicable legal or regulatory
              requirement, and claims arising under consumer protection,
              privacy or similar legislation.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Intellectual property rights.
              </strong>{" "}
              In the event of any third-party claim that the app or your
              possession and use of it infringes that third party&apos;s
              intellectual property rights, {PUBLISHER}, not Apple, will be
              solely responsible for the investigation, defence, settlement and
              discharge of that claim.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Legal compliance.
              </strong>{" "}
              You represent and warrant that you are not located in a country
              subject to a U.S. Government embargo or designated as a
              &ldquo;terrorist supporting&rdquo; country, and that you are not
              listed on any U.S. Government list of prohibited or restricted
              parties.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Developer name and address.
              </strong>{" "}
              Questions, complaints and claims about the app should be directed
              to {PUBLISHER}, {POSTAL_ADDRESS.join(", ")},{" "}
              <MailLink email={CONTACT_EMAIL} />.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Third-party terms.
              </strong>{" "}
              You must comply with any applicable third-party terms when using
              the app.
            </>,
            <>
              <strong className="font-medium text-neutral-100">
                Third-party beneficiary.
              </strong>{" "}
              Apple and its subsidiaries are third-party beneficiaries of this
              agreement and, upon your acceptance of it, will have the right —
              and will be deemed to have accepted the right — to enforce it
              against you as a third-party beneficiary.
            </>,
          ]}
        />
      </Section>

      <Section id="law" heading="11. Governing law">
        <p>
          These terms are governed by the law of {JURISDICTION}, and its courts
          have exclusive jurisdiction, except that if you are a consumer you may
          also bring proceedings in the courts of the country where you live.
        </p>
      </Section>

      <Section id="contact" heading="12. Contact">
        <p>
          {PUBLISHER}, {POSTAL_ADDRESS.join(", ")} —{" "}
          <MailLink email={CONTACT_EMAIL} />. See also our{" "}
          <In href="/privacy">Privacy Policy</In> and the{" "}
          <In href="/support">support page</In>.
        </p>
      </Section>
    </LegalPage>
  );
}
