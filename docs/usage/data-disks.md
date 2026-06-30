# Data Disks

The **Data Disks** tab (press ++2++ or ++tab++) manages persistent disks that live
independently of your VMs. Keep datasets, results, and anything else you can't afford to
lose on a data disk — destroying a VM never touches them.

## Why data disks

vmup VMs are designed to be disposable: spin one up for a job, destroy it when done. The
boot disk dies with the VM. A data disk persists across that cycle — destroy the VM on
Friday, launch a bigger one on Monday, attach the same disk, and your data is right
where you left it.

## The disk list

Disks are listed with their name, zone, type, size, status, and which VMs they're
attached to. Navigation, filtering (++slash++), and refresh (++r++) work exactly like
the [instance list](instances.md).

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-title">vmup - 1.6.2 - GCP Instance Manager</span>
 
 <span class="t-tab">1 Instances</span> <span class="t-tab-active">2 Data Disks</span>                             <span class="t-dim">refreshed 3:04:05 PM</span>
 
  <span class="t-header">Disk Name        Project          Size     Zone            Type          Status     Attached To</span>
  <span class="t-dim">────────────────────────────────────────────────────────────────────────────────────────────────</span>
<span class="t-selected">&gt; reference-data   my-gcp-project   100 GB   us-central1-a   pd-balanced   <span class="t-running">READY</span>      rstudio-demo</span>
  scratch-disk     my-gcp-project   50 GB    us-central1-a   pd-ssd        <span class="t-running">READY</span>      —
  archive-disk     my-gcp-project   500 GB   us-central1-a   pd-standard   <span class="t-orange">CREATING</span>   —
  <span class="t-dim">────────────────────────────────────────────────────────────────────────────────────────────────</span>
 
  <span class="t-key">Disk Name:</span>   reference-data
  <span class="t-key">Zone:</span>        us-central1-a
  <span class="t-key">Type:</span>        pd-balanced
  <span class="t-key">Size:</span>        100 GB
  <span class="t-key">Status:</span>      <span class="t-running">READY</span>
  <span class="t-key">Attached To:</span> rstudio-demo
 
  <span class="t-dim">↑/↓/←/→ navigate • : command • / filter • r refresh • ? help</span></pre>
</div>

## Disk operations

| Key | Action | Notes |
| --- | --- | --- |
| ++n++ | Create | New persistent disk: name, project, zone, type, size |
| ++shift+i++ | Import | Bring an existing GCP disk under vmup management |
| ++e++ | Resize | Grow a disk (GCP disks cannot shrink) |
| ++a++ | Attach | Attach to a running VM and mount it |
| ++d++ | Detach | Detach from one VM, or from all VMs at once |
| ++shift+d++ | Delete | Two-step confirmation; only possible while detached |

### Creating a disk

Press ++n++ on the Data Disks tab. You choose:

- **Disk type** — `pd-standard` (cheapest), `pd-balanced` (default), or `pd-ssd`
  (fastest)
- **Size** — default 50 GB; you can grow it later with ++e++
- **Zone** — a disk can only attach to VMs in the same zone

vmup provisions the disk with Terraform and formats it on first attach.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">Create New Data Disk</span>
 
  <span class="t-header">Disk Name</span>
  <span class="t-dim">Must be lowercase, no underscores</span>
  <span class="t-focus">reference-data▎          </span>
 
  <span class="t-header">Project ID</span>
  <span class="t-dim">GCP project to create the disk in</span>
  <span class="t-input">my-gcp-project           </span>
 
  <span class="t-header">Zone</span>
  <span class="t-input">us-central1-a            </span>
 
  <span class="t-header">Disk Type</span>
  pd-balanced (Balanced SSD) <span class="t-dim">▼</span>
 
  <span class="t-header">Disk Size (GB)</span>
  <span class="t-dim">Minimum 10 GB</span>
  <span class="t-input">100                      </span>
 
  <span class="t-btn">✓ Submit</span>  <span class="t-btn-dim">Cancel</span>
 
  <span class="t-dim">esc/ctrl+c cancel</span></pre>
</div>

### Attaching and mounting

Press ++a++ from either tab — attach a disk to the selected VM, or a VM to the selected
disk. vmup attaches the disk, formats it if it's brand new, and mounts it at your chosen
mount path on the VM.

A disk can be attached to **multiple VMs in read-only mode**, which is handy for sharing
a reference dataset across a fleet of workers. Read-write attachment is exclusive to one
VM.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">Attach Disk to: rstudio-demo</span>
 
  <span class="t-dim">Project: my-gcp-project • Zone: us-central1-a</span>
 
  <span class="t-header">Disk</span>
  <span class="t-dim">Select a managed data disk to attach</span>
  reference-data (100 GB, pd-balanced) <span class="t-dim">▼</span>
 
  <span class="t-header">Mode</span>
  <span class="t-dim">Read/Write is exclusive to one VM. Read-Only allows multiple VMs to share.</span>
  Read-Only (shareable across VMs) <span class="t-dim">▼</span>
 
  <span class="t-header">Mount after attaching?</span>
  <span class="t-running">(•)</span> Yes, configure mount
  ( ) No, attach only
 
  <span class="t-header">Mount Point</span>
  <span class="t-dim">Directory path where the disk will be mounted</span>
  <span class="t-input">/mnt/disks/reference-data</span>
 
  <span class="t-btn">✓ Submit</span>  <span class="t-btn-dim">Cancel</span>
 
  <span class="t-dim">esc/ctrl+c cancel</span></pre>
</div>

For brand-new (unformatted) disks attached read-write, the mount step also offers a
**Format disk?** choice (ext4 or xfs) and an owner for the mount point.

### Detaching

Press ++d++ to detach — from a single VM or from all VMs at once. Detach before deleting
a disk or destroying its zone's resources.

## Attachment status from the Instances tab

The instance detail pane shows attached data disks, and the attach/detach keys
(++a++/++d++) work from the Instances tab too, operating on the selected VM.

!!! tip "Disks survive everything except ++shift+d++ on the disk itself"
    Stopping, restarting, even destroying VMs leaves data disks intact. The only way to
    lose one is to explicitly delete it from the Data Disks tab — which requires it to be
    detached and asks you to type its name to confirm.
