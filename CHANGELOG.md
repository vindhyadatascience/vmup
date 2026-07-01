# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.8.0] - 2026-07-01

### Added
- The Configure New VM form selects regions, zones, and machine types
  dynamically from the Compute API instead of hard-coded lists, and filters
  machine types to the selected image's CPU architecture (ARM64/x86_64).
- A review/confirmation step before a VM is created or updated: it shows a
  summary and requires an explicit yes, so a stray Enter on the last field
  can no longer launch a VM. Backing out (Esc or No) returns to the form with
  everything entered still in place.

### Changed
- Favorited (custom-project) images are now listed alphabetically.
- SSH readiness now always waits with the full timeout and reports the
  underlying gcloud/SSH error when it times out.

### Fixed
- The Region selector shows the default region (us-central1) selected on first
  render instead of scrolling to the top of the list.
- The selected image is now recorded reliably; the picker could previously
  fall back to the default image.
- Images available in both the custom project and the public image list are no
  longer shown twice.

## [1.7.2] - 2026-06-30

### Fixed
- Terraform could fail to install on first run with `unable to verify checksums
  signature: openpgp: key expired`. Upgraded `hc-install` (v0.9.2 → v0.9.5),
  which bundles a current HashiCorp signing key.

## [1.7.1] - 2026-06-30

### Fixed
- The version shown in the app's title bar now matches the release version. It's
  derived from the build version (the same source as `vmup --version`, injected
  from the git tag) instead of a hand-maintained constant, so it can no longer
  drift from the release.

## [1.7.0] - 2026-06-30

First open-source release.

### Added
- Apache-2.0 license, and standard open-source project files (CONTRIBUTING,
  SECURITY, CODE_OF_CONDUCT, issue/PR templates).
- Configurable image source: an optional "image project" setting lists custom
  images above the standard public GCP images when creating a VM, with automatic
  fallback when the configured project isn't accessible.
- Branded documentation site (Material for MkDocs), published to GitHub Pages.
- `vmup --version` flag, with the version stamped into release builds.
- Release artifacts now include a checksums file, and the release pipeline
  supports optional macOS code signing and notarization.

### Changed
- The source image project and the IAP access email domain are no longer
  hard-coded; the username and email domain are derived from the active gcloud
  account.
- The module path is now `github.com/vindhyadatascience/vmup` so the tool can be
  installed with `go install`.
- Install scripts no longer require GitHub authentication.

For releases prior to the open-source release, see the
[GitHub releases](https://github.com/vindhyadatascience/vmup/releases) page
(tags `v1.0.0` through `v1.6.2`).
