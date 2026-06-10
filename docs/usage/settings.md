# Settings

Press ++comma++ to open the settings screen.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">Settings</span>
 
  <span class="t-header">Data Directory</span>
  <span class="t-dim">Where VM projects and disks are stored</span>
  <span class="t-focus">/Users/eric/.vmup▎               </span>
 
  <span class="t-btn">✓ Submit</span>  <span class="t-btn-dim">Cancel</span>
 
  <span class="t-dim">esc cancel</span></pre>
</div>

When you change the directory, vmup shows what lives at the source and destination and
offers to **migrate** your existing projects and disks or just **switch**, with a review
screen before anything moves.

## Data directory

By default vmup stores everything under `~/.vmup`:

```
~/.vmup/
├── bin/                  # auto-installed Terraform binary
├── projects/<vm-name>/   # per-VM Terraform state and variables
├── disks/<disk-name>/    # per-disk Terraform state and variables
└── settings.json         # vmup settings
```

The settings screen lets you point `projects/` and `disks/` at a custom location — for
example a synced or backed-up directory, or a larger volume. The setting is persisted in
`~/.vmup/settings.json`:

```json
{
  "data_dir": "/path/to/custom/dir"
}
```

!!! warning "Moving existing state"
    The Terraform state files under `projects/` and `disks/` are how vmup tracks your
    cloud resources. When changing the data directory, prefer **Migrate & switch** so
    that state moves with it — with **Switch only**, vmup won't see resources whose
    state stayed behind (they keep running in GCP regardless; see
    [Troubleshooting](../reference/troubleshooting.md#vmup-lost-track-of-a-vm)).
