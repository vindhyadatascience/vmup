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

## Copying files

Press ++t++ on a running instance to open the transfer form. Pick a direction,
a local path, and a path on the VM; vmup runs:

```bash
gcloud compute scp --tunnel-through-iap <source> <destination>
```

This takes the same IAP path as SSH, so it needs no extra setup — the
`roles/iap.tunnelResourceAccessor` grant and the firewall rule that already
allow you to connect also allow you to copy. `--recurse` is applied
automatically when the local path is a directory.

## Using scp, rsync, and sftp directly

`gcloud compute scp` cannot resume an interrupted copy or sync only what
changed. For large or repeated transfers, open a **transfer tunnel** instead:
press ++:++ and run `transfer-tunnel`. vmup opens a raw IAP tunnel to the
instance's SSH port and shows the exact commands to use it, for example:

```bash
scp -P 2222 -i ~/.ssh/google_compute_engine -o HostKeyAlias=my-vm \
    ./local-file eric@localhost:~/

rsync -av --progress \
    -e 'ssh -p 2222 -i ~/.ssh/google_compute_engine -o HostKeyAlias=my-vm' \
    ./local-dir/ eric@localhost:~/local-dir/
```

Three details the generated commands handle for you:

- **`-i ~/.ssh/google_compute_engine`** — gcloud manages its own SSH key, which
  is not the identity `ssh` would pick by default.
- **`-o HostKeyAlias=<vm-name>`** — every VM reached this way looks like
  `localhost:<port>` to SSH. Without the alias, connecting to a second VM on
  the same port trips `REMOTE HOST IDENTIFICATION HAS CHANGED`.
- **The port** — vmup picks the first free port at or above 2222.

The same tunnel works for anything that speaks SSH, including `sftp`, GUI
clients like Cyberduck, and VS Code Remote-SSH.

Transfer tunnels are listed alongside service tunnels in the instance list and
are closed when vmup exits. Service tunnels are deliberately different: they
survive so that a long-running RStudio or Jupyter session stays reachable
between runs.

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
