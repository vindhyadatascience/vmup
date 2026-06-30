# Command Palette & Keys

Everything in vmup is reachable from the keyboard. Most actions have a single-key
shortcut, and all of them are discoverable through the command palette.

## The command palette

Press ++colon++ anywhere in the main view to open the palette. Type to fuzzy-filter
commands by name or description, use ++left++/++right++ to cycle category tabs, and
++enter++ to run the highlighted command.

Typing filters the list live, and ++left++/++right++ jumps between categories — the loop
below shows both:

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<div class="vmup-anim-stage">
<pre class="vmup-terminal-body vmup-frame vmup-frame-1"><span class="t-dim">──────────────────────────────────────────────────────────────────</span>
  <span class="t-key">:</span> <span class="t-cursor">▎</span>
<span class="t-dim">──────────────────────────────────────────────────────────────────</span>
 
  <span class="t-tab-active">All</span> <span class="t-tab">Create &amp; Connect</span> <span class="t-tab">Modify &amp; Inspect</span> <span class="t-tab">Stop &amp; Destroy</span> <span class="t-tab">Utility</span>
 
<span class="t-selected">&gt; <span class="t-running">new-instance</span> <span class="t-dim">(n)</span>        Create a new VM instance</span>
  <span class="t-running">start-instance</span> <span class="t-dim">(s)</span>      Start VM &amp; connect tunnels
  <span class="t-running">connect</span> <span class="t-dim">(c)</span>             Connect through SSH
  <span class="t-info">edit-instance</span> <span class="t-dim">(e)</span>       Edit VM configuration
  <span class="t-info">info</span> <span class="t-dim">(i)</span>                View VM info
  <span class="t-info">attach-disk</span> <span class="t-dim">(a)</span>         Attach disk to VM
  <span class="t-stopped">stop-instance</span> <span class="t-dim">(x)</span>       Stop VM
  <span class="t-stopped">destroy-instance</span> <span class="t-dim">(D)</span>    Destroy VM
  filter <span class="t-dim">(/)</span>              Filter list
  <span class="t-dim">↓ more</span>
 
<span class="t-dim">↑/↓ navigate • ←/→ category • enter run • esc close</span></pre>
<pre class="vmup-terminal-body vmup-frame vmup-frame-2"><span class="t-dim">──────────────────────────────────────────────────────────────────</span>
  <span class="t-key">:</span> d<span class="t-cursor">▎</span>
<span class="t-dim">──────────────────────────────────────────────────────────────────</span>
 
  <span class="t-tab-active">All</span> <span class="t-tab">Create &amp; Connect</span> <span class="t-tab">Modify &amp; Inspect</span> <span class="t-tab">Stop &amp; Destroy</span> <span class="t-tab">Utility</span>
 
<span class="t-selected">&gt; <span class="t-info">edit-instance</span> <span class="t-dim">(e)</span>       Edit VM configuration</span>
  <span class="t-info">attach-disk</span> <span class="t-dim">(a)</span>         Attach disk to VM
  <span class="t-info">detach-disk</span> <span class="t-dim">(d)</span>         Detach disk from VM
  <span class="t-stopped">destroy-instance</span> <span class="t-dim">(D)</span>    Destroy VM
 
<span class="t-dim">↑/↓ navigate • ←/→ category • enter run • esc close</span></pre>
<pre class="vmup-terminal-body vmup-frame vmup-frame-3"><span class="t-dim">──────────────────────────────────────────────────────────────────</span>
  <span class="t-key">:</span> di<span class="t-cursor">▎</span>
<span class="t-dim">──────────────────────────────────────────────────────────────────</span>
 
  <span class="t-tab-active">All</span> <span class="t-tab">Create &amp; Connect</span> <span class="t-tab">Modify &amp; Inspect</span> <span class="t-tab">Stop &amp; Destroy</span> <span class="t-tab">Utility</span>
 
<span class="t-selected">&gt; <span class="t-info">edit-instance</span> <span class="t-dim">(e)</span>       Edit VM configuration</span>
  <span class="t-info">attach-disk</span> <span class="t-dim">(a)</span>         Attach disk to VM
  <span class="t-info">detach-disk</span> <span class="t-dim">(d)</span>         Detach disk from VM
 
<span class="t-dim">↑/↓ navigate • ←/→ category • enter run • esc close</span></pre>
<pre class="vmup-terminal-body vmup-frame vmup-frame-4"><span class="t-dim">──────────────────────────────────────────────────────────────────</span>
  <span class="t-key">:</span> dis<span class="t-cursor">▎</span>
<span class="t-dim">──────────────────────────────────────────────────────────────────</span>
 
  <span class="t-tab-active">All</span> <span class="t-tab">Create &amp; Connect</span> <span class="t-tab">Modify &amp; Inspect</span> <span class="t-tab">Stop &amp; Destroy</span> <span class="t-tab">Utility</span>
 
<span class="t-selected">&gt; <span class="t-info">attach-disk</span> <span class="t-dim">(a)</span>         Attach disk to VM</span>
  <span class="t-info">detach-disk</span> <span class="t-dim">(d)</span>         Detach disk from VM
 
<span class="t-dim">↑/↓ navigate • ←/→ category • enter run • esc close</span></pre>
<pre class="vmup-terminal-body vmup-frame vmup-frame-5"><span class="t-dim">──────────────────────────────────────────────────────────────────</span>
  <span class="t-key">:</span> <span class="t-cursor">▎</span>
<span class="t-dim">──────────────────────────────────────────────────────────────────</span>
 
  <span class="t-tab">All</span> <span class="t-tab-active">Create &amp; Connect</span> <span class="t-tab">Modify &amp; Inspect</span> <span class="t-tab">Stop &amp; Destroy</span> <span class="t-tab">Utility</span>
 
<span class="t-selected">&gt; <span class="t-running">new-instance</span> <span class="t-dim">(n)</span>        Create a new VM instance</span>
  <span class="t-running">start-instance</span> <span class="t-dim">(s)</span>      Start VM &amp; connect tunnels
  <span class="t-running">connect</span> <span class="t-dim">(c)</span>             Connect through SSH
 
<span class="t-dim">↑/↓ navigate • ←/→ category • enter run • esc close</span></pre>
<pre class="vmup-terminal-body vmup-frame vmup-frame-6"><span class="t-dim">──────────────────────────────────────────────────────────────────</span>
  <span class="t-key">:</span> <span class="t-cursor">▎</span>
<span class="t-dim">──────────────────────────────────────────────────────────────────</span>
 
  <span class="t-tab">All</span> <span class="t-tab">Create &amp; Connect</span> <span class="t-tab">Modify &amp; Inspect</span> <span class="t-tab-active">Stop &amp; Destroy</span> <span class="t-tab">Utility</span>
 
<span class="t-selected">&gt; <span class="t-stopped">stop-instance</span> <span class="t-dim">(x)</span>       Stop VM</span>
  <span class="t-stopped">stop-all</span> <span class="t-dim">(X)</span>            Stop all VMs &amp; tunnels
  <span class="t-stopped">destroy-instance</span> <span class="t-dim">(D)</span>    Destroy VM
 
<span class="t-dim">↑/↓ navigate • ←/→ category • enter run • esc close</span></pre>
</div>
</div>

### Create & Connect

| Key | Command | Description |
| --- | --- | --- |
| ++n++ | `new-instance` | Create a new VM instance (or a new disk on the Data Disks tab) |
| ++s++ | `start-instance` | Start a VM and connect its tunnels |
| ++c++ | `connect` | Open an SSH session (running VMs only) |
| ++shift+i++ | `import-disk` | Import an existing disk (Data Disks tab) |

### Modify & Inspect

| Key | Command | Description |
| --- | --- | --- |
| ++e++ | `edit-instance` | Edit a VM's configuration / resize a disk |
| ++i++ | `info` | View VM details |
| ++a++ | `attach-disk` | Attach a disk to a VM |
| ++d++ | `detach-disk` | Detach a disk from a VM |

### Stop & Destroy

| Key | Command | Description |
| --- | --- | --- |
| ++x++ | `stop-instance` | Stop tunnels for a VM, optionally stop the VM |
| ++shift+x++ | `stop-all` | Stop all tunnels and VMs |
| ++shift+d++ | `destroy-instance` | Destroy a VM / delete a disk (two-step confirm) |

### Utility

| Key | Command | Description |
| --- | --- | --- |
| ++slash++ | `filter` | Filter the list (free text or `property:value`) |
| ++r++ | `refresh` | Reload the list from GCP |
| ++p++ | `progress` | View the streaming log of the current or last operation |
| ++tab++ | `switch-tab` | Toggle between Instances and Data Disks |
| ++comma++ | `settings` | Open settings |
| ++q++ | `quit` | Exit vmup |

## Navigation keys

| Keys | Action |
| --- | --- |
| ++up++ / ++down++ or ++j++ / ++k++ | Move through the list |
| ++left++ / ++right++ or ++h++ / ++l++ | Switch tabs |
| ++tab++ / ++shift+tab++ | Next / previous tab |
| ++1++ / ++2++ | Jump straight to Instances / Data Disks |
| ++question++ | Toggle the help dialog |
| ++esc++ | Back / cancel / close dialog |
| ++ctrl+c++ | Cancel form, or quit |
| ++ctrl+d++ | Force quit from any screen |

## The help dialog

Press ++question++ on any list screen for an in-app cheat sheet of the current tab's
bindings:

<div class="vmup-terminal">
<div class="vmup-terminal-bar"><span></span><span></span><span></span></div>
<pre class="vmup-terminal-body">  <span class="t-key">╭──────────────────────────────────────╮</span>
  <span class="t-key">│</span> <span class="t-key">Commands</span>                             <span class="t-key">│</span>
  <span class="t-key">│</span>                                      <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">↑/↓/j/k</span>     Navigate list           <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">←/→/h/l</span>     Switch tabs             <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">n</span>           Create a new VM         <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">e</span>           Edit VM configuration   <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">s</span>           Start VM &amp; connect      <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">c</span>           Connect through SSH     <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">x</span>           Stop VM                 <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">D</span>           Destroy VM              <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">:</span>           Command palette         <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-key">/</span>           Filter list             <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-dim">⋮</span>                                    <span class="t-key">│</span>
  <span class="t-key">│</span>  <span class="t-dim">Press any key to close</span>              <span class="t-key">│</span>
  <span class="t-key">╰──────────────────────────────────────╯</span></pre>
</div>

!!! tip
    The in-app help dialog (++question++) always shows the bindings for the tab you're
    on — it's the quickest reference while working.
