# Security Policy

## Supported versions

vmup is distributed as release binaries. Security fixes are applied to the
latest release; please ensure you are running the most recent version before
reporting an issue.

## Reporting a vulnerability

Please **do not** open a public issue for security vulnerabilities.

Instead, report privately via either:

- GitHub's [private vulnerability reporting](https://github.com/vindhyadatascience/vmup/security/advisories/new)
  (the **Security** tab → **Report a vulnerability**), or
- email **info@vindhyadatascience.com** with the details.

Please include:

- a description of the vulnerability and its impact,
- steps to reproduce or a proof of concept,
- the vmup version (`vmup --version`) and your OS.

We will acknowledge your report, investigate, and coordinate a fix and
disclosure timeline with you. Please give us a reasonable opportunity to
address the issue before any public disclosure.

## Scope notes

vmup runs Terraform and `gcloud` using **your** local credentials and creates
resources in **your** GCP project. It does not transmit credentials anywhere.
Provisioned VMs receive a generated password written to the instance; review
the embedded Terraform in `assets/` to understand exactly what is created.
