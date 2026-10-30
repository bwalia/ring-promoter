#!/usr/bin/env python3
"""Turn image CVE scans (SpectonCR, Grype) into Ring Promoter scans + metrics.

SpectonCR (../spectoncr) scans every image pushed to it against OSV and serves
the result at GET /v2/scan/live/{digest} (spectoncr_scan.sh pushes the image
and saves that JSON). Grype scans the same image as a cross-check: SpectonCR
does not yet see JARs inside a Spring Boot fat jar, PHP built from source, or
Alpine packages (it queries OSV by bare apk purl, which OSV never matches), so
on its own it reads a 12-critical image as clean. Each scanner is reported
separately, so the dashboard shows exactly what each one found.

For every scanner given this writes:

  * <rp-dir>/<scanner>.json  the body for POST /api/security/scans (kind
                             "container"), so Ring Promoter's non-overridable
                             security gate judges the exact version;
  * stdout                   Prometheus metrics for the rp-qa Pushgateway, read
                             by the "Ring Promoter — CVE Scans" dashboard and by
                             each app's Grafana gate.

    python3 cve_metrics.py --app hello-php --version 0.3.0-0de4760 \
        --spectoncr spectoncr.json --grype grype.json \
        --report-url https://… --rp-dir out/ > cve.prom

The CVE policy is the one the security gate enforces (promotion_policy.
security.max): NO-GO when critical > --max-critical, high > --max-high, or
minor (medium + low) > --max-minor. Defaults 0, 0, 20. Unrated findings count
as medium and Grype's "negligible" as low.

Metrics (labelled app="<app>", scanner="spectoncr"|"grype"):
    rp_cve_findings{severity}             CVE counts (critical/high/medium/low)
    rp_cve_minor_findings                 medium + low
    rp_cve_fixable_findings{severity}     CVEs with a fixed version available
    rp_cve_policy_max{bucket}             the limits (critical/high/minor)
    rp_cve_gate_status                    2 = within policy, 0 = blocks promotion
    rp_cve_scan_timestamp_seconds         when the scan finished
    rp_cve_packages{ecosystem}            packages SpectonCR found in the image
    rp_cve_top{id,package,installed,fixed,severity}  CVSS of the worst 25 CVEs
    rp_cve_scan_info{version,image,digest,report_url}  always 1

Only the standard library is used.
"""

import argparse
import json
import os
import sys
import time
from collections import Counter
from datetime import datetime, timezone

SEVERITIES = ("critical", "high", "medium", "low")
RANK = {s: i for i, s in enumerate(SEVERITIES)}
ALIAS = {"negligible": "low"}
TOP_N = 25


def esc(v):
    return str(v).replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"')


def labels(**kv):
    return "{" + ",".join(f'{k}="{esc(v)}"' for k, v in kv.items()) + "}"


def severity(v):
    s = str(v or "").lower()
    s = ALIAS.get(s, s)
    return s if s in RANK else "medium"


def epoch(ts):
    """RFC 3339 UTC (SpectonCR writes nanoseconds and Z) -> Unix seconds."""
    if not ts:
        return time.time()
    # Drop the fraction: fromisoformat before 3.11 takes neither 9 digits nor Z.
    return datetime.fromisoformat(ts.rstrip("Z").split(".")[0] + "+00:00").timestamp()


def finding(id_, sev, title, pkg, installed, fixed, cvss):
    return {"id": id_, "severity": severity(sev), "title": (title or "")[:200], "package": pkg or "",
            "installed": installed or "", "fixed": fixed or "", "cvss": cvss}


def from_spectoncr(path):
    with open(path) as f:
        doc = json.load(f)
    if doc.get("status") != "completed":
        sys.exit(f"{path}: spectoncr scan not completed (status={doc.get('status')!r})")
    res = doc.get("result") or {}
    out = [finding(v.get("id"), v.get("severity"), v.get("summary"), v.get("package"),
                   v.get("installed_version"), v.get("fixed_version"), v.get("cvss_score"))
           for v in res.get("vulnerabilities") or [] if v.get("id")]
    # SpectonCR reads every layer's package DB, so a package upgraded in a
    # later layer is listed twice; count distinct packages.
    pkgs = {(p.get("ecosystem") or "unknown", p.get("name"), p.get("version")) for p in res.get("packages") or []}
    return out, Counter(e for e, _, _ in pkgs), epoch(res.get("completed_at")), res.get("digest", "")


def from_grype(path):
    with open(path) as f:
        doc = json.load(f)
    out = []
    for m in doc.get("matches") or []:
        v, a = m.get("vulnerability") or {}, m.get("artifact") or {}
        scores = [c.get("metrics", {}).get("baseScore") for c in v.get("cvss") or []]
        scores = [s for s in scores if isinstance(s, (int, float))]
        fixed = (v.get("fix") or {}).get("versions") or []
        out.append(finding(v.get("id"), v.get("severity"), v.get("description"), a.get("name"),
                           a.get("version"), fixed[0] if fixed else "", max(scores) if scores else None))
    digest = ((doc.get("source") or {}).get("target") or {}).get("manifestDigest", "")
    ts = (doc.get("descriptor") or {}).get("timestamp")
    # Grype's JSON lists only vulnerable packages, so no package inventory.
    return out, Counter(), epoch(ts[:19] + "Z") if ts else time.time(), digest


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n", 1)[0])
    ap.add_argument("--app", required=True)
    ap.add_argument("--version", required=True)
    ap.add_argument("--image", default="", help="image reference scanned (shown on the dashboard)")
    ap.add_argument("--spectoncr", help="SpectonCR GET /v2/scan/live/{digest} JSON")
    ap.add_argument("--grype", help="grype -o json output")
    ap.add_argument("--report-url", default="", help="SpectonCR scan report URL")
    ap.add_argument("--run-url", default="", help="CI run URL (Grype evidence)")
    ap.add_argument("--rp-dir", help="write POST /api/security/scans bodies here, one per scanner")
    ap.add_argument("--max-critical", type=int, default=0)
    ap.add_argument("--max-high", type=int, default=0)
    ap.add_argument("--max-minor", type=int, default=20)
    a = ap.parse_args()
    if not (a.spectoncr or a.grype):
        ap.error("give --spectoncr and/or --grype")

    scans = []
    if a.spectoncr:
        scans.append(("spectoncr", "SpectonCR (OSV)", a.report_url or a.run_url, *from_spectoncr(a.spectoncr)))
    if a.grype:
        scans.append(("grype", "Grype", a.run_url, *from_grype(a.grype)))

    # name -> (help, sample lines). Samples of one family must be contiguous
    # in the exposition format, so every scanner's rows are gathered first.
    families = {}

    def metric(name, help_, base, rows):
        fam = families.setdefault(name, (help_, []))
        fam[1].extend(f"{name}{labels(**base, **extra)} {value}" for extra, value in rows)

    blocks = []
    for scanner, label, url, raw, eco, scanned, digest in scans:
        # One finding per (advisory, package, version): a CVE in two packages
        # is two things to fix; the same one listed twice is one.
        findings, seen = [], set()
        for f in raw:
            key = (f["id"], f["package"], f["installed"])
            if key not in seen:
                seen.add(key)
                findings.append(f)
        findings.sort(key=lambda f: (RANK[f["severity"]], -(f["cvss"] or 0), f["id"]))
        counts = Counter(f["severity"] for f in findings)
        fixable = Counter(f["severity"] for f in findings if f["fixed"])
        minor = counts["medium"] + counts["low"]
        breaches = []
        if counts["critical"] > a.max_critical:
            breaches.append(f"{counts['critical']} critical > max {a.max_critical}")
        if counts["high"] > a.max_high:
            breaches.append(f"{counts['high']} high > max {a.max_high}")
        if minor > a.max_minor:
            breaches.append(f"{minor} minor > max {a.max_minor}")
        summary = (f"{label} {a.image or a.app}: {counts['critical']} critical, {counts['high']} high, "
                   f"{counts['medium']} medium, {counts['low']} low"
                   + (f" across {sum(eco.values())} packages" if eco else ""))
        blocks.append((summary, breaches))

        if a.rp_dir:
            os.makedirs(a.rp_dir, exist_ok=True)
            with open(os.path.join(a.rp_dir, f"{scanner}.json"), "w") as f:
                json.dump({
                    "app": a.app, "version": a.version, "kind": "container", "scanner": scanner,
                    "url": url, "summary": summary,
                    "scanned_at": datetime.fromtimestamp(scanned, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                    "findings": [{k: x[k] for k in ("id", "severity", "title", "package", "installed", "fixed")}
                                 for x in findings],
                }, f)

        base = {"app": a.app, "scanner": scanner}
        metric("rp_cve_findings", "Image CVEs by scanner and severity.", base,
               [({"severity": s}, counts[s]) for s in SEVERITIES])
        metric("rp_cve_minor_findings", "Medium + low image CVEs (the 'minor' policy bucket).", base, [({}, minor)])
        metric("rp_cve_fixable_findings", "Image CVEs with a fixed version available, by severity.", base,
               [({"severity": s}, fixable[s]) for s in SEVERITIES])
        metric("rp_cve_policy_max", "Most CVEs the promotion policy allows per bucket.", base,
               [({"bucket": "critical"}, a.max_critical), ({"bucket": "high"}, a.max_high),
                ({"bucket": "minor"}, a.max_minor)])
        metric("rp_cve_gate_status", "2 when the image CVEs are within policy, 0 when they block promotion.",
               base, [({}, 0 if breaches else 2)])
        metric("rp_cve_scan_timestamp_seconds", "Unix time the image CVE scan finished.", base,
               [({}, int(scanned))])
        if eco:
            metric("rp_cve_packages", "Packages the scanner found in the image, by ecosystem.", base,
                   [({"ecosystem": e}, n) for e, n in sorted(eco.items())])
        metric("rp_cve_top", "CVSS score of the most severe image CVEs (0 when unscored).", base,
               [({"id": f["id"], "package": f["package"], "installed": f["installed"],
                  "fixed": f["fixed"] or "none", "severity": f["severity"]}, f["cvss"] or 0)
                for f in findings[:TOP_N]])
        metric("rp_cve_scan_info", "The image a CVE scan covered.", base,
               [({"version": a.version, "image": a.image, "digest": digest,
                  "report_url": url}, 1)])

    for name, (help_, samples) in families.items():
        print(f"# HELP {name} {help_}\n# TYPE {name} gauge")
        if samples:
            print("\n".join(samples))
    for summary, breaches in blocks:
        print(f"{summary} -> {'NO-GO (' + ', '.join(breaches) + ')' if breaches else 'GO'}", file=sys.stderr)


if __name__ == "__main__":
    main()
