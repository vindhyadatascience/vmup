package main

import "embed"

//go:embed assets/main.tf
var embeddedMainTF string

//go:embed assets
var assetsFS embed.FS
