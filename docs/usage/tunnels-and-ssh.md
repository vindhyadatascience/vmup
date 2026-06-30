# Tunnels & SSH

vmup never exposes your instances to the public internet. All access — interactive SSH
and forwarded services like RStudio — goes through
[Identity-Aware Proxy (IAP)](https://cloud.google.com/iap/docs/using-tcp-forwarding)
tunnels, authenticated with your Google identity.

## How tunneling works

Under the hood vmup runs:

```bash
gcloud compute ssh <vm-name> --tunnel-through-iap -- -L <local>:localhost:<remote>
```

The firewall on every vmup-created VPC only accepts traffic from Google's IAP range
(`35.235.240.0/20`), so the tunnel is the *only* way in. Your Google account needs the
`roles/iap.tunnelResourceAccessor` role on the instance — vmup's Terraform grants this
to you automatically at launch.

## Port mappings

Port mappings are configured per VM in the launch form as comma-separated
`local:remote` pairs:

```
8787:8787, 8888:8888
```

Each pair becomes its own SSH tunnel: the service listening on `remote` on the VM is
reachable at `localhost:<local>` on your machine. The default `8787:8787` maps RStudio
Server. Firewall rules allow remote ports 80, 443, 2000–2999, and 8000–9999, which
covers RStudio, Jupyter, Shiny, and most development servers.

## Tunnel lifecycle

- **After launch** — tunnels start automatically once the VM accepts SSH (vmup polls
  until it's ready).
- **Start** (++s++) — starting a stopped VM re-establishes all its tunnels.
- **Stop** (++x++) — closes the VM's tunnels; you can optionally stop the VM too.
- **Stop all** (++shift+x++) — closes every tunnel and stops all VMs.

The instance list shows the live tunnel count next to each running VM, and the status
screen lists each tunnel's local URL.

## Interactive SSH

Press ++c++ on a running instance to open a full SSH session through IAP. The TUI
suspends while the session is active and resumes when you exit the shell.

## Setting up the VM after connecting

A couple of one-time steps make a fresh VM more useful:

**Git / GitHub credentials**

```bash
gh auth login
```

**Docker access to GitHub Container Registry**

Create a [classic personal access token](https://github.com/settings/tokens) with the
`read:packages` scope, save it to `~/.ghcr_token` on the VM, then:

```bash
cat ~/.ghcr_token | docker login ghcr.io -u <username> --password-stdin
```
