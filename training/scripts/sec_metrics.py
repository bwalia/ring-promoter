#!/usr/bin/env python3
"""Turn security scan output into Prometheus metrics for the AppSec gate.

The same evidence Ring Promoter's own security gate judges per version (SARIF
uploaded to POST /api/security/scans) is also published here as metrics, so
grafana.workstation.co.uk shows the posture of the latest build and Ring
Promoter's Grafana gate can turn it into Go/No-Go — exactly like the unit-test
metrics in qa_unit_metrics.py.

    python3 sec_metrics.py --app hello-world --stage source --version sha-abc1234 \
        --sarif sast=gosec.sarif --sarif sast=semgrep.sarif --sarif sca=govulncheck.sarif \
        --sarif secrets=gitleaks.sarif --sarif iac=checkov.sarif \
        --checkov-json checkov.json --checkov-config .checkov.yaml > source.prom

    python3 sec_metrics.py --app hello-world --stage image --version sha-abc1234 \
        --sbom sbom.cdx.json --license-sarif-out license.sarif \
        --sarif container=grype.sarif > image.prom

--license-sarif-out evaluates the licence policy against the SBOM and writes the
result as SARIF (kind "license"), so it can be uploaded like any other scan; it
is also counted in the metrics.

Severity follows Ring Promoter's SARIF mapping exactly (internal/promoter/
sarif.go): security-severity >= 9 critical, >= 7 high, >= 4 medium, else low;
without a score, level error -> high, note/none -> low, anything else ->
medium; every secrets finding is critical. A kind is GO (2) with no critical or
high finding, NO-GO (0) otherwise — the same max {critical: 0, high: 0} the
security policy enforces.

Metrics (all labelled app="<app>"; the Pushgateway adds stage="<stage>"):
    rp_sec_scan_status{kind}                  2 = no critical/high, 0 = blocked
    rp_sec_findings{kind,scanner,severity}    finding counts
    rp_sec_scan_timestamp_seconds{kind}       when the kind was scanned
    rp_sec_last_run_timestamp_seconds         when this stage ran
    rp_sec_compliance_checks{framework,result}  Checkov passed/failed/suppressed
    rp_sec_compliance_score_percent           passed / (passed + failed) * 100
    rp_sec_sbom_present                       1 when an SBOM was generated
    rp_sec_sbom_components{type}              SBOM components by type
    rp_sec_sbom_licenses{license}             third-party libraries by licence
    rp_sec_supply_chain_control{control}      1 = in place (e.g. provenance), 0 = missing
    rp_sec_run_info{version,commit,run_url}   always 1; carries the build

Only the standard library is used.
"""

import argparse
import json
import re
import sys
import time

SEVERITIES = ("critical", "high", "medium", "low")
KINDS = ("sast", "sca", "secrets", "container", "iac", "dast", "license")

# Strong copyleft and source-available licences that cannot ship inside a
# proprietary or SaaS build without legal review. Matched against SPDX ids.
DENY = re.compile(r"^(AGPL|GPL|SSPL|EUPL|OSL|CPAL|RPL|BUSL|Commons-Clause)", re.I)
# Packages that are the platform, not a dependency: OS packages in the base
# image (their licences cover distribution of the OS itself), the Go standard
# library, and the application's own module.
PLATFORM_PURL = re.compile(r"^pkg:(deb|apk|rpm|alpm)/|^pkg:golang/stdlib@")


def esc(v):
    return str(v).replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"')


def labels(**kv):
    return "{" + ",".join(f'{k}="{esc(v)}"' for k, v in kv.items()) + "}"


def score(props):
    v = (props or {}).get("security-severity")
    try:
        return float(str(v).strip()) if v is not None else None
    except ValueError:
        return None


def from_score(s):
    return "critical" if s >= 9 else "high" if s >= 7 else "medium" if s >= 4 else "low"


def from_level(level):
    level = (level or "").lower()
    return "high" if level == "error" else "low" if level in ("note", "none") else "medium"


def sarif_findings(path, kind):
    """Return (scanner, [severity, ...]) for one SARIF file."""
    with open(path, encoding="utf-8") as f:
        log = json.load(f)
    tool, sevs = "", []
    for run in log.get("runs") or []:
        driver = (run.get("tool") or {}).get("driver") or {}
        tool = tool or driver.get("name", "")
        rules = driver.get("rules") or []
        by_id = {r.get("id"): r for r in rules}
        for res in run.get("results") or []:
            rule = by_id.get(res.get("ruleId"))
            idx = res.get("ruleIndex")
            if rule is None and isinstance(idx, int) and 0 <= idx < len(rules):
                rule = rules[idx]
            rule = rule or {}
            s = score(res.get("properties"))
            if s is None:
                s = score(rule.get("properties"))
            if s is not None:
                sev = from_score(s)
            else:
                level = res.get("level") or (rule.get("defaultConfiguration") or {}).get("level")
                sev = from_level(level)
            if kind == "secrets":
                sev = "critical"
            sevs.append(sev)
    return tool or "unknown", sevs


def component_licenses(c):
    out = []
    for l in c.get("licenses") or []:
        lic = l.get("license") or {}
        name = lic.get("id") or lic.get("name") or l.get("expression")
        if name:
            out.append(name)
    return out


def license_policy(sbom, app):
    """Evaluate the licence policy; return (sarif dict, {licence: count})."""
    rules, results, counts = [], [], {}
    for c in sbom.get("components") or []:
        purl = c.get("purl") or ""
        if c.get("type") != "library" or not purl or PLATFORM_PURL.search(purl):
            continue
        if "@" not in purl:
            continue  # the application's own main module carries no version
        lics = component_licenses(c)
        for lic in lics or ["UNKNOWN"]:
            counts[lic] = counts.get(lic, 0) + 1
        name = f"{c.get('name')}@{c.get('version', '')}"
        denied = [l for part in lics for l in re.split(r"\s+(?:AND|OR|WITH)\s+|[(),]", part) if l and DENY.search(l)]
        if denied:
            rid = f"license-denied/{denied[0]}"
            level, text = "error", f"{name} is licensed {', '.join(lics)}: {denied[0]} is not allowed in a shipped build"
        elif not lics:
            rid, level, text = "license-unknown", "warning", f"{name} declares no licence; review before release"
        else:
            continue
        if rid not in {r["id"] for r in rules}:
            rules.append({"id": rid, "shortDescription": {"text": rid.replace("/", ": ")},
                          "defaultConfiguration": {"level": level}})
        results.append({"ruleId": rid, "level": level, "message": {"text": text},
                        "locations": [{"physicalLocation": {"artifactLocation": {"uri": purl}}}]})
    sarif = {"version": "2.1.0", "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
             "runs": [{"tool": {"driver": {"name": "rp-license-policy", "rules": rules}}, "results": results}]}
    return sarif, counts


def checkov_summary(path):
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    reports = data if isinstance(data, list) else [data]
    out = {}
    for r in reports:
        s = r.get("summary") or {}
        fw = r.get("check_type", "unknown")
        prev = out.get(fw, {"passed": 0, "failed": 0})
        out[fw] = {"passed": prev["passed"] + int(s.get("passed", 0)),
                   "failed": prev["failed"] + int(s.get("failed", 0))}
    return out


def suppressed_checks(path):
    """Count the skip-check entries in a Checkov config (no YAML parser needed)."""
    n, inside = 0, False
    with open(path, encoding="utf-8") as f:
        for line in f:
            stripped = line.split("#", 1)[0].rstrip()
            if not stripped:
                continue
            if re.match(r"^skip-check\s*:", stripped):
                inside = True
                continue
            if inside:
                if re.match(r"^\s+-\s*\S", stripped):
                    n += 1
                elif not stripped.startswith(" "):
                    inside = False
    return n


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--app", required=True)
    ap.add_argument("--stage", required=True, help="source | image (one Pushgateway group each)")
    ap.add_argument("--sarif", action="append", default=[], metavar="KIND=PATH")
    ap.add_argument("--checkov-json")
    ap.add_argument("--checkov-config")
    ap.add_argument("--sbom", help="CycloneDX JSON SBOM")
    ap.add_argument("--license-sarif-out", help="write the licence-policy result as SARIF")
    ap.add_argument("--control", action="append", default=[], metavar="NAME=0|1",
                    help="a supply-chain control and whether it is in place")
    ap.add_argument("--version", default="")
    ap.add_argument("--commit", default="")
    ap.add_argument("--run-url", default="")
    args = ap.parse_args()

    app, now = args.app, int(time.time())
    findings = {}  # (kind, scanner) -> {severity: n}
    kinds = set()

    def add(kind, path):
        if kind not in KINDS:
            sys.exit(f"unknown scan kind {kind!r} (want one of {', '.join(KINDS)})")
        scanner, sevs = sarif_findings(path, kind)
        bucket = findings.setdefault((kind, scanner), dict.fromkeys(SEVERITIES, 0))
        for s in sevs:
            bucket[s] += 1
        kinds.add(kind)

    for spec in args.sarif:
        kind, _, path = spec.partition("=")
        add(kind, path)

    sbom, lic_counts = None, {}
    if args.sbom:
        with open(args.sbom, encoding="utf-8") as f:
            sbom = json.load(f)
        if args.license_sarif_out:
            sarif, lic_counts = license_policy(sbom, app)
            with open(args.license_sarif_out, "w", encoding="utf-8") as f:
                json.dump(sarif, f, indent=2)
            add("license", args.license_sarif_out)

    out = []

    def metric(name, help_, rows):
        out.append(f"# HELP {name} {help_}")
        out.append(f"# TYPE {name} gauge")
        out.extend(f"{name}{lbl} {val}" for lbl, val in rows)

    status = {}
    for k in sorted(kinds):
        blocking = sum(b["critical"] + b["high"] for (kk, _), b in findings.items() if kk == k)
        status[k] = 0 if blocking else 2
    metric("rp_sec_scan_status", "2 when a scan kind has no critical or high finding, 0 when it blocks.",
           [(labels(app=app, kind=k), v) for k, v in status.items()])
    metric("rp_sec_findings", "Security findings by scan kind, scanner and severity.",
           [(labels(app=app, kind=k, scanner=s, severity=sev), n)
            for (k, s), b in sorted(findings.items()) for sev, n in b.items()])
    metric("rp_sec_scan_timestamp_seconds", "Unix time a scan kind last ran.",
           [(labels(app=app, kind=k), now) for k in sorted(kinds)])
    metric("rp_sec_last_run_timestamp_seconds", "Unix time this scan stage last ran.",
           [(labels(app=app), now)])

    if args.checkov_json:
        cs = checkov_summary(args.checkov_json)
        sup = suppressed_checks(args.checkov_config) if args.checkov_config else 0
        rows = []
        for fw, s in sorted(cs.items()):
            rows += [(labels(app=app, framework=fw, result="passed"), s["passed"]),
                     (labels(app=app, framework=fw, result="failed"), s["failed"])]
        rows.append((labels(app=app, framework="all", result="suppressed"), sup))
        metric("rp_sec_compliance_checks", "Checkov CIS benchmark checks by framework and result.", rows)
        passed = sum(s["passed"] for s in cs.values())
        ran = passed + sum(s["failed"] for s in cs.values())
        metric("rp_sec_compliance_score_percent", "Compliance checks passed / ran * 100.",
               [(labels(app=app), round(100.0 * passed / ran, 2) if ran else 0)])

    if sbom is not None:
        types = {}
        for c in sbom.get("components") or []:
            types[c.get("type", "unknown")] = types.get(c.get("type", "unknown"), 0) + 1
        metric("rp_sec_sbom_present", "1 when a CycloneDX SBOM was generated for the build.",
               [(labels(app=app), 1)])
        metric("rp_sec_sbom_components", "SBOM components by type.",
               [(labels(app=app, type=t), n) for t, n in sorted(types.items())])
        if lic_counts:
            metric("rp_sec_sbom_licenses", "Third-party libraries in the SBOM by licence.",
                   [(labels(app=app, license=l), n) for l, n in sorted(lic_counts.items())])

    if args.control:
        rows = []
        for spec in args.control:
            name, _, val = spec.partition("=")
            rows.append((labels(app=app, control=name), 1 if val.strip() in ("1", "true") else 0))
        metric("rp_sec_supply_chain_control", "1 when a supply-chain control is in place for the build, 0 when missing.", rows)

    metric("rp_sec_run_info", "The build this scan stage covered.",
           [(labels(app=app, version=args.version, commit=args.commit[:12], run_url=args.run_url), 1)])

    print("\n".join(out))
    for (k, s), b in sorted(findings.items()):
        print(f"{app} {k} ({s}): " + " ".join(f"{n}{sev[0].upper()}" for sev, n in b.items())
              + ("  NO-GO" if status[k] == 0 else "  GO"), file=sys.stderr)


if __name__ == "__main__":
    main()
