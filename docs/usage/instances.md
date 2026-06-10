# Managing Instances

The **Instances** tab is vmup's home screen: a live table of every compute instance in
your project, with one-key actions for the full VM lifecycle.

## The instance list

Instances are listed with their name, project, zone, machine type, and status
(`RUNNING`, `STOPPED`, and transitional states). Running VMs also show how many active
tunnels they have. A detail pane below the table shows the selected VM's image, port
mappings, and boot disk size.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-title">vmup - 1.6.2 - GCP Instance Manager</span>
 
 <span class="t-tab-active">1 Instances</span> <span class="t-tab">2 Data Disks</span>                              <span class="t-dim">refreshed 3:04:05 PM</span>
 
  <span class="t-header">VM Name        Project          Zone            Machine         Status</span>
  <span class="t-dim">───────────────────────────────────────────────────────────────────────────</span>
<span class="t-selected">&gt; rstudio-eric   my-gcp-project   us-central1-a   e2-highmem-2    <span class="t-running">RUNNING (1 tunnel)</span></span>
  analysis-vm    my-gcp-project   us-central1-a   e2-highmem-4    <span class="t-stopped">STOPPED</span>
  batch-runner   my-gcp-project   us-central1-a   e2-standard-4   <span class="t-orange">PROVISIONING</span>
  <span class="t-dim">───────────────────────────────────────────────────────────────────────────</span>
 
  <span class="t-key">Image:</span>         vds-debian-13-base
  <span class="t-key">Port Mapping:</span>  8787:8787
  <span class="t-key">Username:</span>      eric
  <span class="t-key">Boot Disk:</span>     20 GB
  <span class="t-key">Data Disks:</span>    reference-data (100 GB, ro)
  <span class="t-info">Tunnel active: http://localhost:8787 (PID 52114)</span>
 
  <span class="t-dim">↑/↓/←/→ navigate • : command • / filter • r refresh • ? help</span></pre>
</div>

- Navigate with ++up++/++down++ (or ++j++/++k++)
- ++r++ refreshes the list from GCP
- On narrow terminals the table collapses into a card layout automatically

## Filtering

Press ++slash++ to filter the list. Two styles are supported:

- **Free text** — matches anywhere: `rstudio`
- **Property search** — `status:running`, `zone:us-central1-a`, `machine:e2-highmem-2`,
  `name:analysis`

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">/</span> status:running<span class="t-cursor">▎</span>
 
<span class="t-dim">tab next • enter apply • esc cancel</span></pre>
</div>

After applying, the help bar shows the active filter and match count —
<code>filter: status running (1/3) • / edit • esc clear</code>. The filter is remembered
per tab, so switching to Data Disks and back keeps your filter.

## Lifecycle actions

All actions work on the selected instance, either by direct key or through the
[command palette](command-palette-and-keys.md) (++colon++):

| Key | Action | What happens |
| --- | --- | --- |
| ++n++ | New instance | Opens the launch form ([walkthrough](../getting-started/first-vm.md)) |
| ++e++ | Edit | Reopens the form for an existing VM — change machine type, ports, disk size, then re-apply |
| ++i++ | Info | Detailed view of the VM's configuration |
| ++s++ | Start | Starts a stopped VM and re-establishes its SSH tunnels |
| ++c++ | Connect | Interactive SSH session via IAP (running VMs only) |
| ++x++ | Stop | Closes tunnels, and optionally stops the VM to save costs |
| ++shift+x++ | Stop all | Stops every VM and all tunnels |
| ++shift+d++ | Destroy | Runs `terraform destroy` and removes all the VM's infrastructure |
| ++p++ | Progress | Re-opens the streaming log of the current or last operation |

## Destroying an instance

Destroying is a **two-step confirmation**: confirm the prompt, then type the VM's name
exactly. vmup runs `terraform destroy`, tearing down the VM, its VPC, NAT, and firewall
rules, then deletes the local state directory under `~/.vmup/projects/<vm-name>/`.

!!! warning "Boot disk contents are deleted"
    Destroy removes the boot disk and everything on it. Keep anything you care about on
    a [persistent data disk](data-disks.md) — those survive a VM destroy.

## Editing an instance

Press ++e++ to edit a VM's configuration. vmup reopens the launch form pre-filled with
the current values; on submit it re-runs `terraform apply` to reconcile the changes.
Changing the machine type requires the VM to restart.
