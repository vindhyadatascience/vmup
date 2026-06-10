# Prerequisites

vmup keeps its own dependencies to a minimum — Terraform is downloaded automatically on
first run, so there are only two things you need before installing.

## 1. Google Cloud SDK

The [`gcloud` CLI](https://cloud.google.com/sdk/docs/install) is required for IAP SSH
tunneling — vmup shells out to `gcloud compute ssh --tunnel-through-iap` to reach your
instances.

After installing, authenticate:

```bash
gcloud auth login
```

vmup will also auto-detect your default project from `gcloud config`, so it helps to set
one:

```bash
gcloud config set project YOUR_PROJECT_ID
```

!!! tip "Application Default Credentials"
    Some features (live cost estimates, batch instance queries) use the Google Cloud APIs
    directly. If you see authentication errors inside vmup, run
    `gcloud auth application-default login`. vmup will prompt you when this is needed.

## 2. GitHub access

vmup is distributed from a **private** GitHub repository, so the installer needs to
authenticate as someone with access to
[`vindhyadatascience/vds-gcp-launch-instance`](https://github.com/vindhyadatascience/vds-gcp-launch-instance).
Either of these works:

- **GitHub CLI (recommended)** — install the [`gh` CLI](https://cli.github.com) and run
  `gh auth login` once.
- **Personal access token** — a token with `repo` scope, exported as `GITHUB_TOKEN`.

## That's it

You do **not** need to install:

- **Terraform** — vmup downloads its own pinned copy to `~/.vmup/bin/` on first run.
- **Go** — only needed if you want to [build from source](installation.md#from-source).

Next: [install vmup](installation.md).
