#!/usr/bin/env python3
"""Turn `go test -json` output into Prometheus metrics for the QA gate.

Ring Promoter's Grafana gate reads these from grafana.workstation.co.uk (the
obs Prometheus scrapes them off the rp-qa Pushgateway). One run replaces the
app's whole group on the Pushgateway, so a deleted test disappears instead of
keeping its last result forever.

    go test -json -coverprofile=cover.out ./... > test.json
    python3 qa_unit_metrics.py --app hello-world --json test.json \
        --cover cover.out --version v1.4.0 --commit "$GITHUB_SHA" > metrics.prom

Metrics (all labelled app="<app>"):
    rp_qa_unit_last_run_status            2 = every test passed, 0 = a failure
    rp_qa_unit_last_run_timestamp_seconds when the run finished (staleness)
    rp_qa_unit_tests{result}              passed / failed / skipped counts
    rp_qa_unit_tests_total                tests that ran (skips excluded)
    rp_qa_unit_pass_rate_percent          passed / (passed + failed) * 100
    rp_qa_unit_coverage_percent           statement coverage
    rp_qa_unit_duration_seconds           wall time of the whole run
    rp_qa_unit_test_result{package,test}  2 pass, 1 skip, 0 fail — per test
    rp_qa_unit_run_info{version,commit,run_url}  always 1; carries the build

Only the standard library is used: the runner needs nothing installed.
"""

import argparse
import json
import sys
import time

RESULT_VALUE = {"pass": 2, "skip": 1, "fail": 0}


def esc(v):
    return str(v).replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"')


def labels(**kv):
    return "{" + ",".join(f'{k}="{esc(v)}"' for k, v in kv.items()) + "}"


def parse_tests(path):
    """Return ({(package, test): action}, build_failed, elapsed)."""
    results, build_failed, elapsed = {}, False, 0.0
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line.startswith("{"):
                continue
            ev = json.loads(line)
            action = ev.get("Action")
            pkg, test = ev.get("Package", ""), ev.get("Test")
            if action not in ("pass", "fail", "skip"):
                if action == "build-fail":
                    build_failed = True
                continue
            if test:
                results[(pkg, test)] = action
            else:
                # Package-level result: a failure here with no failing test
                # means the package did not compile or crashed outside a test.
                elapsed += float(ev.get("Elapsed") or 0)
                if action == "fail" and not any(
                    p == pkg and a == "fail" for (p, _), a in results.items()
                ):
                    build_failed = True
    return results, build_failed, elapsed


def parse_coverage(path):
    """Statement coverage from a Go coverprofile, or None."""
    if not path:
        return None
    covered = total = 0
    seen = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.startswith("mode:") or not line.strip():
                continue
            block, stmts, count = line.rsplit(" ", 2)
            # -coverpkg can list a block more than once; it counts once.
            prev = seen.get(block)
            hit = int(count) > 0
            if prev is None:
                seen[block] = (int(stmts), hit)
            elif hit and not prev[1]:
                seen[block] = (prev[0], True)
    for stmts, hit in seen.values():
        total += stmts
        covered += stmts if hit else 0
    return 100.0 * covered / total if total else None


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--app", required=True)
    ap.add_argument("--json", required=True, help="go test -json output")
    ap.add_argument("--cover", help="coverprofile from -coverprofile")
    ap.add_argument("--version", default="")
    ap.add_argument("--commit", default="")
    ap.add_argument("--run-url", default="")
    args = ap.parse_args()

    results, build_failed, elapsed = parse_tests(args.json)
    passed = sum(1 for a in results.values() if a == "pass")
    failed = sum(1 for a in results.values() if a == "fail")
    skipped = sum(1 for a in results.values() if a == "skip")
    ran = passed + failed
    # No tests at all is a failure, not a vacuous pass: a gate must never go
    # GO because the suite was accidentally deleted or stopped compiling.
    ok = not build_failed and failed == 0 and passed > 0
    cover = parse_coverage(args.cover)
    app = args.app

    out = []

    def metric(name, help_, typ, rows):
        out.append(f"# HELP {name} {help_}")
        out.append(f"# TYPE {name} {typ}")
        for lbl, val in rows:
            out.append(f"{name}{lbl} {val}")

    metric("rp_qa_unit_last_run_status", "2 if every unit test passed, 0 on any failure.", "gauge",
           [(labels(app=app), 2 if ok else 0)])
    metric("rp_qa_unit_last_run_timestamp_seconds", "Unix time the unit-test run finished.", "gauge",
           [(labels(app=app), int(time.time()))])
    metric("rp_qa_unit_tests", "Unit tests by result.", "gauge",
           [(labels(app=app, result=r), n) for r, n in
            (("passed", passed), ("failed", failed), ("skipped", skipped))])
    metric("rp_qa_unit_tests_total", "Unit tests that ran (skips excluded).", "gauge",
           [(labels(app=app), ran)])
    metric("rp_qa_unit_pass_rate_percent", "Passed / ran * 100.", "gauge",
           [(labels(app=app), round(100.0 * passed / ran, 2) if ran else 0)])
    if cover is not None:
        metric("rp_qa_unit_coverage_percent", "Statement coverage of the unit tests.", "gauge",
               [(labels(app=app), round(cover, 2))])
    metric("rp_qa_unit_duration_seconds", "Wall time of the unit-test run.", "gauge",
           [(labels(app=app), round(elapsed, 3))])
    metric("rp_qa_unit_test_result", "Per test: 2 pass, 1 skip, 0 fail.", "gauge",
           [(labels(app=app, package=p, test=t), RESULT_VALUE[a])
            for (p, t), a in sorted(results.items())])
    metric("rp_qa_unit_run_info", "The build the last run tested.", "gauge",
           [(labels(app=app, version=args.version, commit=args.commit[:12], run_url=args.run_url), 1)])

    print("\n".join(out))
    print(f"{app}: {passed} passed, {failed} failed, {skipped} skipped"
          + (f", coverage {cover:.1f}%" if cover is not None else "")
          + (" (BUILD FAILED)" if build_failed else ""), file=sys.stderr)


if __name__ == "__main__":
    main()
