# Releasing vmup

Releases are automated with [GoReleaser](https://goreleaser.com/) and GitHub
Actions. Pushing a `vX.Y.Z` tag triggers `.github/workflows/release.yml`, which
cross-compiles the binaries, builds the archives + checksums, and publishes a
GitHub Release.

```bash
git tag v1.7.0
git push origin v1.7.0
```

macOS binaries are currently shipped **unsigned**. That's fine for the primary
install path — `install.sh` doesn't quarantine binaries, so they run as-is; only
users who download a release archive in a browser hit Gatekeeper, which they can
clear with `xattr -d com.apple.quarantine ./vmup`. To enable Developer ID signing
+ notarization, see [Enabling macOS signing](#enabling-macos-signing-optional).

## Enabling macOS signing (optional)

Signing is **not wired into the release pipeline by default** — it requires both
the config changes in Step 4 and the five secrets below. quill signs and
notarizes the macOS binaries on the Linux runner, so no macOS runner is needed.

You need a **paid Apple Developer Program** membership. Creating a Developer ID
certificate requires the **Account Holder** role (individual accounts already
have it).

You will produce five GitHub Actions secrets:

| Secret | What it is |
| --- | --- |
| `QUILL_SIGN_P12` | base64 of your Developer ID Application cert (`.p12`) |
| `QUILL_SIGN_PASSWORD` | the password you set when exporting the `.p12` |
| `QUILL_NOTARY_KEY` | contents of your App Store Connect API key (`.p8`) |
| `QUILL_NOTARY_KEY_ID` | the API key's Key ID |
| `QUILL_NOTARY_ISSUER` | the API key's Issuer ID |

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
   a `.p12` and set a strong password (this becomes `QUILL_SIGN_PASSWORD`).
5. Base64-encode the `.p12` for the secret:
   ```bash
   base64 -i DeveloperID.p12 | pbcopy   # now in your clipboard → QUILL_SIGN_P12
   ```

### Step 2 — App Store Connect API key (notarization)

1. Go to <https://appstoreconnect.apple.com/access/integrations/api> (App Store
   Connect → **Users and Access** → **Integrations** → **App Store Connect
   API**, **Team Keys**).
2. Note the **Issuer ID** shown at the top → `QUILL_NOTARY_ISSUER`.
3. Click **+**, give the key a name, set **Access: Developer** (or higher),
   generate it.
4. Note the new key's **Key ID** → `QUILL_NOTARY_KEY_ID`.
5. **Download the `.p8`** (you can only download it once). Its full contents
   (including the `-----BEGIN PRIVATE KEY-----` lines) become `QUILL_NOTARY_KEY`.

### Step 3 — Add the secrets to GitHub

In the repo: **Settings ▸ Secrets and variables ▸ Actions ▸ New repository
secret**, add all five secrets from the table above.

### Step 4 — Wire signing into the release

Add a quill install step to `.github/workflows/release.yml` (before the "Run
GoReleaser" step) and pass the secrets as env to GoReleaser:

```yaml
      - name: Install quill
        run: curl -sSfL https://raw.githubusercontent.com/anchore/quill/main/install.sh | sh -s -- -b /usr/local/bin

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          QUILL_SIGN_P12: ${{ secrets.QUILL_SIGN_P12 }}
          QUILL_SIGN_PASSWORD: ${{ secrets.QUILL_SIGN_PASSWORD }}
          QUILL_NOTARY_KEY: ${{ secrets.QUILL_NOTARY_KEY }}
          QUILL_NOTARY_KEY_ID: ${{ secrets.QUILL_NOTARY_KEY_ID }}
          QUILL_NOTARY_ISSUER: ${{ secrets.QUILL_NOTARY_ISSUER }}
```

Add a `binary_signs` block to `.goreleaser.yml` that signs only the macOS
binaries, then **validate it with `goreleaser check`** before tagging:

```yaml
binary_signs:
  - cmd: quill
    args: ["sign-and-notarize", "${artifact}", "-vv"]
    if: '{{ eq .Os "darwin" }}'
    output: true
```

!!! note
    The `if` filter on `binary_signs` requires a recent GoReleaser — older
    versions (e.g. v2.16) reject it with *"field if not found in type
    config.BinarySign"*. If `goreleaser check` fails on it, pin a newer
    GoReleaser in the action (`version:`) or restrict signing by build `ids`
    (define a darwin-only build id and reference it via `ids:`) instead.

> Creating the Developer ID certificate (Step 1) requires the Apple Developer
> account's **Account Holder** role — it cannot be done by Admins. If you are not
> the Account Holder, generate the CSR yourself and have the Account Holder
> create the certificate from it, so the private key stays with you.

## Local testing

With [GoReleaser](https://goreleaser.com/install/) and `quill` installed:

```bash
goreleaser release --snapshot --clean   # ad-hoc signs, no Apple credentials
goreleaser check                        # validate .goreleaser.yml
```

## Verifying a signed binary

After downloading a released macOS binary:

```bash
codesign -dv --verbose=4 ./vmup    # shows the Developer ID authority
spctl -a -vvv -t install ./vmup    # Gatekeeper assessment (bare binaries may
                                   # report "rejected" but still run once
                                   # notarization is recognized online)
```
