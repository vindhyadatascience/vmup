# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
