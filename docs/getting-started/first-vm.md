# Your First VM

This walkthrough takes you from a fresh install to a running instance with RStudio open
in your browser.

## 1. Start vmup

```bash
vmup
```

vmup opens on the **Instances** tab. On first run the list will be empty. If you aren't
authenticated with Google Cloud yet, vmup offers to run `gcloud auth login` for you.

## 2. Create a new instance

Press ++n++ (or open the command palette with ++colon++ and choose `new-instance`). The
launch form appears with sensible defaults already filled in:

| Field | Default | Notes |
| --- | --- | --- |
| Project ID | auto-detected from `gcloud config` | The GCP project to deploy into |
| VM name | — | Lowercase letters, numbers, and hyphens |
| Image | first available image | Listed from your configured image project, then the standard public GCP images |
| Region / Zone | `us-central1` / `us-central1-a` | Chosen from live lists fetched from GCP; the zone options update to match the selected region |
| Machine type | `e2-highmem-2` | Filtered to the image's CPU architecture (ARM64/x86_64); a live **hourly cost estimate** is shown as you choose |
| Boot disk size | `20` GB | |
| Port mapping | `8787:8787` | Comma-separated `local:remote` pairs |

!!! note "This example uses a custom RStudio image"
    The screens below show a custom `my-rstudio-image` (an image with RStudio
    preinstalled) surfaced through an
    [image-project setting](../reference/configuration.md#settings). Without a custom
    image project configured, the picker lists the standard public GCP images (Debian,
    Ubuntu, etc.) — pick any of those to follow along.

The cost estimate comes from the Cloud Billing API (with built-in fallback rates), so you
see roughly what the machine costs per hour before anything is created.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">Configure New VM</span>
 
  <span class="t-header">Project ID</span>
  <span class="t-dim">GCP project to create the instance in</span>
  <span class="t-input">my-gcp-project           </span>
 
  <span class="t-header">VM Name</span>
  <span class="t-dim">Must be lowercase, no underscores</span>
  <span class="t-focus">rstudio-demo▎            </span>
 
  <span class="t-header">Image</span>
  my-rstudio-image <span class="t-dim">▼</span>
 
  <span class="t-header">Region</span>
  us-central1 <span class="t-dim">▼</span>
 
  <span class="t-header">Zone</span>
  us-central1-a <span class="t-dim">▼</span>
 
  <span class="t-header">Machine Type</span>
<span class="t-selected">&gt; ★ e2-highmem-2 (2 vCPU, 16 GB)   <span class="t-running">~$0.12/hr</span></span>
    ★ e2-highmem-4 (4 vCPU, 32 GB)   <span class="t-dim">~$0.24/hr</span>
    ★ e2-standard-2 (2 vCPU, 8 GB)   <span class="t-dim">~$0.08/hr</span>
 
  <span class="t-header">Boot Disk Size (GB)</span>
  <span class="t-dim">OS and system files — destroyed with the VM</span>
  <span class="t-input">20                       </span>
 
  <span class="t-header">Port Mapping</span>
  <span class="t-dim">Comma-separated local:remote (e.g. 8787:8787,2222:22)</span>
  <span class="t-input">8787:8787                </span>
 
  <span class="t-btn">✓ Submit</span>  <span class="t-btn-dim">Cancel</span>
 
  <span class="t-dim">ctrl+c cancel</span></pre>
</div>

## 3. Review and launch

Completing the form opens a **review screen** that summarizes the VM. Nothing is created
until you confirm — press ++y++ (or select **Yes**) to launch, or ++esc++ / **No** to go
back to the form with everything you entered still in place. This makes it hard to create
a VM by accidentally pressing ++enter++.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">Review New VM</span>
 
  <span class="t-key">VM Name:</span>       rstudio-demo
  <span class="t-key">Image:</span>         my-rstudio-image
  <span class="t-key">Image Project:</span> my-image-project
  <span class="t-key">Region:</span>        us-central1
  <span class="t-key">Zone:</span>          us-central1-a
  <span class="t-key">Machine Type:</span>  e2-highmem-2
  <span class="t-key">Boot Disk:</span>     20 GB
  <span class="t-key">Port Mapping:</span>  8787:8787
 
  <span class="t-header">Create this VM?</span>
 
  <span class="t-selected">&gt; Yes</span>    No
 
  <span class="t-dim">←/→ toggle • enter submit • y Yes • n No • esc/ctrl+c back to edit</span></pre>
</div>

Confirm, and vmup runs Terraform for you — `init`, then `apply` — streaming the output
live into the progress screen. Behind the scenes this creates an isolated VPC, NAT,
IAP-only firewall rules, and the instance itself
(see [Infrastructure Created](../reference/infrastructure.md)).

Provisioning typically takes a few minutes. The startup script on the VM also runs
system updates, so allow a couple of extra minutes before everything is responsive.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-orange">◐</span> <span class="t-key">Launching rstudio-demo</span> <span class="t-dim">(1m 12s)</span>
 
  <span class="t-dim">google_compute_network.vpc: Creation complete after 22s</span>
  <span class="t-dim">google_compute_subnetwork.subnet: Creation complete after 19s</span>
  <span class="t-dim">google_compute_router_nat.nat: Creation complete after 11s</span>
  <span class="t-dim">google_compute_instance.main: Creating...</span>
  <span class="t-dim">google_compute_instance.main: Still creating... [20s elapsed]</span>
 
  <span class="t-dim">↑/↓ scroll • ←/→ pan • esc/ctrl+c back</span></pre>
</div>

## 4. Check the status screen

When the apply finishes, the status screen shows:

- Your **username and password** for services on the VM (the password is auto-generated)
- The **SSH tunnels** that were started automatically for each port mapping

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">VM Info</span>
 
  <span class="t-running">Successfully launched rstudio-demo</span>
 
  <span class="t-key">VM Name:</span>       rstudio-demo
  <span class="t-key">Project:</span>       my-gcp-project
  <span class="t-key">Zone:</span>          us-central1-a
  <span class="t-key">Machine:</span>       e2-highmem-2
  <span class="t-key">Image:</span>         my-rstudio-image
  <span class="t-key">Boot Disk:</span>     20 GB
  <span class="t-key">Port Mapping:</span>  8787:8787
  <span class="t-key">Username:</span>      demo
  <span class="t-key">Password:</span>      xR9kL2mP5nQ8vW
 
  <span class="t-info">Active Tunnels:</span>
    <span class="t-info">http://localhost:8787 (PID 52114)</span>
 
  <span class="t-dim">enter/b/esc back • q quit</span></pre>
</div>

## 5. Open your service

With the default port mapping, RStudio is now reachable at
[localhost:8787](http://localhost:8787/). Log in with the credentials from the status
screen.

!!! tip "Change your password"
    The generated password is meant to be temporary. SSH in (++c++ from the instance
    list) and run `sudo passwd <your-username>`.

## 6. When you're done

- ++x++ — stop tunnels, and optionally stop the VM to save money while keeping it around
- ++s++ — start it back up later; tunnels reconnect automatically
- ++shift+d++ — destroy the VM and all its infrastructure when you no longer need it

Continue to [Managing Instances](../usage/instances.md) for the full tour.
