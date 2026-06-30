# Infrastructure Created

Every `vmup` launch provisions a small, fully isolated stack with Terraform. This page
documents exactly what gets created — and the security model behind it.

## Resources per VM

| Resource | Details |
| --- | --- |
| VPC network | Dedicated network per deployment — no shared VPC, no default network |
| Subnetwork | `10.10.0.0/24` in your chosen region |
| Cloud Router + NAT | Outbound internet access (package installs, Docker pulls) without any inbound exposure |
| Firewall: SSH | TCP 22, **only** from Google's IAP range `35.235.240.0/20` |
| Firewall: web | TCP 80, 443, 2000–2999, 8000–9999, **only** from the IAP range |
| IAP API enablement | Enables `iap.googleapis.com` on the project if needed |
| IAP IAM binding | Grants you `roles/iap.tunnelResourceAccessor` on the instance |
| Compute instance | Your chosen image, machine type, and boot disk, plus a startup script |

The startup script creates your user account, sets the generated password, adds the
user to the `docker` group, and applies system updates, then the instance restarts once
to pick everything up.

!!! note "IAP access is granted to your account"
    The IAM binding is created for `<username>@<domain>`. Both default to your active
    gcloud account (e.g. `gcloud config get-value account`), and you can override them on
    the launch form. That Google identity must have access to the project for tunneling
    to work.

## Security model

!!! info "No public ingress, by construction"
    Instances get **no public-facing open ports**. Both firewall rules restrict ingress
    to `35.235.240.0/20` — Google's Identity-Aware Proxy address block. The only way to
    reach a VM is an IAP tunnel authenticated with an authorized Google account.
    Outbound traffic flows through Cloud NAT, so the VM can reach the internet but the
    internet cannot reach the VM.

This means:

- Port scans find nothing — there is no path from the public internet to the VM
- Access control is your Google identity + IAM, not passwords or IP allowlists
- Removing someone's IAP role instantly revokes their access

## Resources per data disk

A [data disk](../usage/data-disks.md) is a single `google_compute_disk` in its own
Terraform project. Disks are created with deletion protection in their Terraform
lifecycle; vmup switches to a deletable configuration only when you explicitly delete
the disk through the two-step confirmation.

## Teardown

`Destroy` (++shift+d++) runs `terraform destroy` on the VM's isolated state: instance,
firewall rules, NAT, router, subnet, and VPC are all removed, and the local state
directory is deleted. Data disks are untouched — they are separate Terraform projects.
