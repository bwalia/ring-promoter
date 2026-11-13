# Play Store assets

Everything the Play listing needs that lives in this repository. The listing
text itself is written in the Console; what is here is what a build produces or
a script draws, so it can be regenerated rather than hunted for.

| What | Where | Regenerate with |
|---|---|---|
| Phone screenshots (7) | `screenshots/` | `../scripts/capture-store-screenshots.sh` |
| Feature graphic (1024×500) | `feature-graphic.png` | `python3 ../scripts/make-feature-graphic.py` |
| App icon | `../app/src/main/res/` | — (adaptive icon, drawn as vectors) |
| Store icon (512×512) | `play-icon-512.png` | `python3 ../scripts/make-play-icon.py` |

The store icon is drawn from the adaptive icon's own numbers, cropped to the
centre 72dp a launcher mask shows, so the Play listing and the home screen
agree. Changing `ic_launcher_foreground.xml` means changing
`make-play-icon.py` and running it again.

The screenshots are captured from **demo mode**, so they show the same data a
reviewer sees when they tap "Explore in demo mode": no real cluster, nothing to
redact, and nothing that goes stale when a server changes.

## Where the listing lives

The app entry was created on 12 November 2026 under the **Cogno AI Developer
Account** (`cognoailtd@gmail.com`, organisation account), not under Workstation
Solutions Ltd, which is who the iOS listing names. Worth knowing before
wondering why the two stores show different publishers.

`org.fictionally.ringpromoter` is now bound to that account. A package name can
never be changed, moved between accounts, reused or freed, so publishing under
a different developer account later would mean a new package name and a new
listing with no install base.

Uploads run as
`ring-promoter-play-publisher@sysops-24-7.iam.gserviceaccount.com`.

## What is not here, and cannot be

The credentials, which are never committed:

1. **The upload key** — `../upload.jks` and `../keystore.properties`, gitignored.
   Whoever owns the listing owns this key; losing it means asking Google to
   reset it, and every build after that is signed by a different key.

   ```sh
   base64 -i upload.jks | pbcopy   # UPLOAD_KEYSTORE_B64
   ```

2. **The Play service account JSON** — `../.android-secrets/`, gitignored.
   CI reads it from Vault at `secret/ring-promoter/android` as
   `PLAY_SERVICE_ACCOUNT_JSON_B64`.

Both belong in that Vault secret. Until they are there they exist on one
machine, in no backup.

Run `../scripts/verify-play-readiness.sh` to see which of these is still
outstanding.

## The data safety form

The app collects nothing. It talks only to the control plane the operator
enters, keeps its token in the device key store, and has no analytics, no
crash reporter and no Firebase. The readiness script checks that no such
dependency has crept in, because the form is a statement about the build, not
about an intention.
