# Releasing vmup

Releases are automated with [GoReleaser](https://goreleaser.com/) and GitHub
Actions. Pushing a `vX.Y.Z` tag triggers `.github/workflows/release.yml`, which
cross-compiles the binaries, signs and notarizes the macOS builds (once the
credentials below exist), builds the archives + checksums, and publishes a
GitHub Release.

```bash
git tag v1.9.0
git push origin v1.9.0
```

## macOS signing and notarization

`.goreleaser.yml` already contains the `notarize.macos` block. It is **inert
until the secrets exist** — `enabled` is evaluated before any credential is
read, so with no secrets set the pipe skips and the release is byte-for-byte
what it would have been. **Adding the five repository secrets below is the only
step required to turn signing on.** There is no config change, no extra
workflow step, and no macOS runner: GoReleaser's notarize pipe is pure Go, it
vendors quill as a library and talks to Apple's notary API over HTTPS from the
existing `ubuntu-latest` runner.

You need a **paid Apple Developer Program** membership. Creating a Developer ID
certificate requires the **Account Holder** role (individual accounts already
have it). A Mac is needed exactly once, to generate the CSR and export the
`.p12` — not to run anything in CI.

| Secret | What it is |
| --- | --- |
| `MACOS_SIGN_P12` | single-line base64 of the Developer ID Application cert (`.p12`) |
| `MACOS_SIGN_PASSWORD` | the password that opens the `.p12` |
| `MACOS_NOTARY_KEY` | single-line base64 of the App Store Connect API key (`.p8`) |
| `MACOS_NOTARY_KEY_ID` | the API key's Key ID |
| `MACOS_NOTARY_ISSUER_ID` | the API key's Issuer UUID |

Both key fields take *base64'd contents* (GoReleaser also accepts a file path,
which is not useful in CI). Encode with no line wrapping and no trailing
newline — on macOS `base64 -i <file>` already emits a single line.

The `.p12` must bundle the **Developer ID Certification Authority (G2)**
intermediate, not just the leaf certificate. A leaf-only export cannot build a
complete chain and fails at signing time.

> **Before adding these secrets**, restrict who can create `v*` tags. The
> release workflow triggers on tag push, and GitHub runs the workflow file
> *from the pushed commit* — so anyone able to create a tag can run arbitrary
> code with access to these credentials. A repository ruleset targeting tags
> `v*` with **Restrict creations** is the control that closes this; branch
> protection on `main` does not.

**Already have a Developer ID Application certificate for the team?** Then
Steps 1 and 2 are done — reuse the existing `.p12` and App Store Connect API
key and skip to Step 3. One certificate covers every product signed by that
team; there is no need for a per-project certificate.

### Step 1 — Developer ID Application certificate (signing)

1. On a Mac, open **Keychain Access** → menu **Keychain Access ▸ Certificate
   Assistant ▸ Request a Certificate From a Certificate Authority**.
   - Enter your email and a Common Name, leave "CA Email" blank, choose
     **Saved to disk**, and save the `.certSigningRequest` (CSR) file.
2. Go to <https://developer.apple.com/account/resources/certificates/list> →
   **+** → choose **Developer ID Application** → upload the CSR → download the
   resulting `.cer`.
3. Double-click the `.cer` to add it to your **login** keychain.
4. In Keychain Access, find **Developer ID Application: \<your name\>**, expand
   it to confirm a private key is attached, right-click → **Export** → save as
   a `.p12` and set a strong password (this becomes `MACOS_SIGN_PASSWORD`).
5. Base64-encode the `.p12` for the secret:
   ```bash
   base64 -i DeveloperID.p12 | pbcopy   # now in your clipboard → MACOS_SIGN_P12
   ```

### Step 2 — App Store Connect API key (notarization)

1. Go to <https://appstoreconnect.apple.com/access/integrations/api> (App Store
   Connect → **Users and Access** → **Integrations** → **App Store Connect
   API**, **Team Keys**).
2. Note the **Issuer ID** shown at the top → `MACOS_NOTARY_ISSUER_ID`.
3. Click **+**, give the key a name, set **Access: Developer** (or higher),
   generate it.
4. Note the new key's **Key ID** → `MACOS_NOTARY_KEY_ID`.
5. **Download the `.p8`** (you can only download it once). Its full contents
   (including the `-----BEGIN PRIVATE KEY-----` lines) become `MACOS_NOTARY_KEY`.

### Step 3 — Add the secrets to GitHub

In the repo: **Settings ▸ Secrets and variables ▸ Actions ▸ New repository
secret**, add all five secrets from the table above. The next tagged release
signs and notarizes automatically.

## Verifying a release

**Do not trust the workflow's exit status.** GoReleaser's notary pipe treats a
notarization *timeout* as a log line, not an error — on a slow day at Apple the
release publishes with signed-but-un-notarized binaries and the job still goes
green. After each release:

1. Search the workflow run log for `notarize timeout`. If present, the macOS
   binaries in that release are not notarized; cut a new patch release.
2. Download and check the published binary:

   ```bash
   codesign -dv --verbose=4 ./vmup   # expect a Developer ID authority
   spctl -a -vvv -t install ./vmup   # expect: accepted, source=Notarized Developer ID
   ```

### Why there is no stapled ticket

A notarization ticket cannot be stapled to a bare Mach-O binary — `stapler`
only attaches tickets to `.app`, `.dmg`, `.pkg` and `.kext` containers. vmup
ships a bare binary in a tarball, so Gatekeeper resolves the ticket with an
**online lookup** instead. In practice this means a browser-downloaded copy is
accepted silently when the machine is online, and can still be refused on a
first run with no network. Stapled `.pkg`/`.dmg` artifacts require GoReleaser
Pro.

### What notarization does and does not fix

Gatekeeper only assesses a file carrying the `com.apple.quarantine` extended
attribute, and only quarantine-aware downloaders set it — browsers, Mail,
AirDrop, and Homebrew **Cask**. `curl` does not. So notarization changes the
experience for exactly one install path: downloading a release archive in a
browser. Users who install via `install.sh`, or who update in place, were never
Gatekeeper-checked to begin with.

## Local testing

With [GoReleaser](https://goreleaser.com/install/) installed:

```bash
goreleaser check                        # validate .goreleaser.yml
goreleaser release --snapshot --clean   # build locally; signing stays skipped
```
