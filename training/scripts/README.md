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
