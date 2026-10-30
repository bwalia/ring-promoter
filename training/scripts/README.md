# Scripts

Helper scripts for the labs.

- [`rp.sh`](./rp.sh) — a thin wrapper over the Ring Promoter API (apps, rings,
  seed, promote, rollback). Set `RP_URL` and `RP_TOKEN`, then:

  ```bash
  chmod +x rp.sh
  export RP_URL=https://ring-promoter.fictionally.org RP_TOKEN=<token>
  ./rp.sh seed hello-world int v0.1.0
  ./rp.sh promote hello-world int
  ./rp.sh promote bank-api acc CR-1234     # gated ring: pass a change-request code
  ```

Gate evidence (used by `.github/workflows/hello-world-qa.yml` and
`polyglot-qa.yml`; standard library only):

- [`qa_unit_metrics.py`](./qa_unit_metrics.py) — unit-test results
  (`go test -json`, or JUnit XML with JaCoCo / Clover coverage) → `rp_qa_unit_*`.
- [`sec_metrics.py`](./sec_metrics.py) — SARIF, SBOM and Checkov results →
  `rp_sec_*`, plus the licence-policy SARIF.
- [`spectoncr_scan.sh`](./spectoncr_scan.sh) — copies an image into SpectonCR
  (acc registry), waits for its OSV scan and saves the result JSON.
- [`cve_metrics.py`](./cve_metrics.py) — SpectonCR and/or Grype results →
  `rp_cve_*` (the "Ring Promoter — CVE Scans" dashboard) and one
  `POST /api/security/scans` body per scanner. Policy: no critical, no high,
  at most 20 minor (medium + low).

  ```bash
  SPECTONCR_USER=… SPECTONCR_PASSWORD=… \
    ./spectoncr_scan.sh docker.io/bwalia/hello-php:0.3.0-0de4760 hello-php 0.3.0-0de4760 sc.json
  grype registry:docker.io/bwalia/hello-php:0.3.0-0de4760 -o json > grype.json
  python3 cve_metrics.py --app hello-php --version 0.3.0-0de4760 \
    --spectoncr sc.json --grype grype.json --rp-dir out/ > cve.prom
  ```
