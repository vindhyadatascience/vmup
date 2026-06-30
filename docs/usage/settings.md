# Settings

Press ++comma++ to open the settings screen.

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body"><span class="t-key">Settings</span>
 
  <span class="t-header">Data Directory</span>
  <span class="t-dim">Where VM projects and disks are stored</span>
  <span class="t-focus">/Users/you/.vmup▎               </span>
 
  <span class="t-header">Image Project (optional)</span>
  <span class="t-dim">GCP project whose images are listed first when creating a VM</span>
  <span class="t-input">my-image-project                </span>
 
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

## Image project

The **Image Project** is an optional GCP project whose images are listed **first** —
above the standard public GCP images, marked with ★ — in the image picker when you
create a VM. Use it to surface your own custom images.

- Leave it **blank** to show only the standard public GCP images (Debian, Ubuntu, etc.).
- Set it to a project you have access to, and its images appear at the top of the picker.

It's persisted in `~/.vmup/settings.json`:

```json
{
  "image_project": "my-image-project"
}
```

!!! note "Access fallback"
    If your Google account can't access the configured image project, vmup shows a
    one-time notice the next time you create a VM, falls back to the standard public
    images, and clears the setting so it won't try again.

### Shipped default: `vds-infrastructure`

Out of the box the Image Project is preset to **`vds-infrastructure`** — the image
project maintained by [Vindhya Data Science](https://vindhyadatascience.com), which hosts
the data-science / RStudio images vmup was originally built for.

- **If you have access** (e.g. Vindhya users), those images appear at the top of the
  picker automatically — nothing to configure.
- **If you don't** (most users), the **first** time you create a VM you'll see a one-time
  *"No access to image project `vds-infrastructure`"* notice. vmup then falls back to the
  standard public GCP images and clears the preset, so every run after that shows only
  the standard images. This is expected and harmless — you don't need to do anything.

Set **Image Project** to a GCP project you have access to (or leave it blank) at any time
to override or remove the default.
