# Configuration & Defaults

## VM launch form

Every field in the launch form, with its default:

| Field | Default | Description |
| --- | --- | --- |
| Project ID | auto-detected from `gcloud config get-value project` | GCP project to deploy into |
| VM name | — | Lowercase letters, numbers, hyphens; also names the local state directory |
| Username | your OS username | Account created on the VM |
| Password | auto-generated (30-char random hex) | For services like RStudio; change it on the VM with `sudo passwd` |
| Image | `vds-debian-13-base` | Source images come from the `vds-infrastructure` project |
| Region | `us-central1` | |
| Zone | `us-central1-a` | |
| Machine type | `e2-highmem-2` | Live hourly cost estimate shown while choosing |
| Boot disk size | `20` GB | |
| Port mapping | `8787:8787` | Comma-separated `local:remote` pairs, one tunnel each |

Values are written to `terraform.tfvars` in the VM's state directory, so ++e++ (edit)
reopens the form with exactly what you launched with.

## Disk creation form

| Field | Default | Description |
| --- | --- | --- |
| Disk name | — | Also names the local state directory |
| Project ID | auto-detected | |
| Zone | `us-central1-a` | Must match the VMs it will attach to |
| Disk type | `pd-balanced` | `pd-standard` \| `pd-balanced` \| `pd-ssd` |
| Size | `50` GB | Can grow later, never shrink |

## Local state layout

```
~/.vmup/
├── bin/
│   └── terraform                 # pinned Terraform, auto-installed on first run
├── projects/
│   └── <vm-name>/
│       ├── main.tf               # embedded Terraform config, written at launch
│       ├── terraform.tfvars      # your form values
│       ├── terraform.tfstate     # state for this VM's infrastructure
│       └── .terraform/           # providers
├── disks/
│   └── <disk-name>/              # same structure per disk
└── settings.json
```

Each VM and each disk is a fully isolated Terraform project — operations on one can
never affect another. The Terraform configuration itself is embedded in the vmup binary
(`go:embed`) and written out at launch time, so there is nothing to keep in sync.

## Cost estimates

When you pick a machine type, vmup shows an estimated hourly price. The estimate is:

1. Fetched live from the **Cloud Billing Catalog API** (per-vCPU and per-GB RAM rates
   for your region, on-demand pricing), keyed by machine family (`e2`, `n1`, `n2`,
   `c2`, …)
2. If the API is unavailable or you lack permission, vmup falls back to **built-in
   rates** (us-central1 on-demand)

Estimates cover the machine only — disks, network egress, and sustained-use discounts
are not included. They are a guide, not a bill.

## Terraform version

vmup installs Terraform 1.12.1 to `~/.vmup/bin/` on first run via HashiCorp's
[hc-install](https://github.com/hashicorp/hc-install). It never uses or modifies a
system-wide Terraform installation.
