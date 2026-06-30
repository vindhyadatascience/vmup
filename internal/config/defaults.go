package config

// DefaultImageProject is the GCP project vmup lists custom images from by
// default. It ships as the initial value of the optional "image project"
// setting (see Settings.EffectiveImageProject): its images are shown above the
// standard public GCP images in the VM-creation picker.
//
// Users without access to this project fall back to the standard public images
// and the setting self-clears (see the access-denied handling in the TUI). It
// is also used as the image project for VM configs created before the
// image-project field existed, so existing projects keep resolving.
//
// This is the one place an org-specific default lives; change it (or set it to
// "") to rebrand vmup for a different organization.
const DefaultImageProject = "vds-infrastructure"
