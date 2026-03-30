variable "disk-name" {
	description = "Name for the data disk"
}

variable "project-id" {
	description = "GCP project ID"
}

variable "zone" {
	default = "us-central1-a"
}

variable "disk-type" {
	default     = "pd-balanced"
	description = "Disk type: pd-standard, pd-balanced, or pd-ssd"
}

variable "disk-size" {
	default     = "50"
	type        = number
	description = "Disk size in GB"
}

variable "formatted" {
	default     = ""
	description = "App metadata: whether the disk has been formatted (not used by Terraform)"
}

provider "google" {
	project = var.project-id
	zone    = var.zone
}

resource "google_compute_disk" "data" {
	name = var.disk-name
	type = var.disk-type
	size = var.disk-size
	zone = var.zone
}
