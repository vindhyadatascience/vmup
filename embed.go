package main

import "embed"

//go:embed assets/main.tf
var embeddedMainTF string

//go:embed assets/disk.tf
var embeddedDiskTF string

//go:embed assets/disk_deletable.tf
var embeddedDiskDeletableTF string

//go:embed assets
var assetsFS embed.FS
