variable "username" {
	description = "Your GCP username (the part before the @ in your email)"
}

variable "password" {
	description = "Configure your user account password (optional)"
}

variable "project-id" {
	description = "Which GCP project ID to create the instance within."
}

variable "vm-name" {
	description = "Name for the VM instance, must be all lowercase with no underscores"
}

variable "image" {
	default = "vds-debian-13-base"
	description = "Which source image should be used (e.g. vds-debian-13-base, vds-debian-13-rstudio-4-5-3, etc.)"
}

variable "region" {
	default = "us-central1"
}

variable "zone" {
	default = "use-central1-a"
}

variable "machine-type" {
	default = "e2-highmem-2"
}

variable "boot-disk-size" {
	default = "20"
	type = number
}

variable "timestamp" {
	description = "Timestamp for assigning instance id and static address name."
}

variable "port-mapping" {
	description = "Comma-separated list of port mappings in local:remote format."
	default = "8787:8787"
}

provider "google" {
	project = var.project-id
	region = var.region
	zone = var.zone
}

resource "google_compute_network" "vpc" {
	name                    = "vpc-${var.timestamp}"
	auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "instance_subnet" {
	name          = "subnet-${var.timestamp}"
	ip_cidr_range = "10.10.0.0/24"
	region        = var.region
	network       = google_compute_network.vpc.self_link
}

resource "google_compute_instance" "default" {
	name = var.vm-name
	machine_type = var.machine-type

	boot_disk {
		auto_delete = true
		device_name = "boot"
		mode = "READ_WRITE"

		initialize_params {
			image = "https://www.googleapis.com/compute/v1/projects/vds-infrastructure/global/images/${var.image}"
			size  = var.boot-disk-size
		}
	}

	network_interface {
		network    = google_compute_network.vpc.self_link
		subnetwork = google_compute_subnetwork.instance_subnet.self_link
	}

	metadata = {
		startup-script = <<-EOF
		#!/bin/bash
		# Add user to docker group
		sudo usermod -aG docker ${var.username}
		sudo apt-get update && sudo apt-get dist-upgrade

		# Generate temporary password for user
		echo "Username=${var.username}" > /home/${var.username}/.env
		echo "Password=${var.password}" >> /home/${var.username}/.env
		echo "${var.username}:${var.password}" | sudo chpasswd
		EOF
	}

	tags = ["http-server", "https-server"]
}

resource "google_compute_firewall" "allow-web" {
	name    = "allow-web-${var.timestamp}"
	network = google_compute_network.vpc.self_link

	allow {
		protocol = "tcp"
		ports    = ["80", "443", "2000-2999", "8000-9999"]
	}

	target_tags   = ["http-server", "https-server"]
	source_ranges = ["35.235.240.0/20"]
}

resource "google_compute_firewall" "allow-ssh-ingress-from-iap" {
	name      = "allow-ssh-ingress-from-iap-${var.timestamp}"
	network   = google_compute_network.vpc.self_link
	direction = "INGRESS"
	allow {
		protocol = "tcp"
		ports    = ["22"]
	}
	source_ranges = ["35.235.240.0/20"]
}

resource "google_compute_router" "nat_router" {
	name    = "nat-router-${var.timestamp}"
	network = google_compute_network.vpc.self_link
	region  = var.region
}

resource "google_compute_router_nat" "nat_config" {
	name                               = "nat-config-${var.timestamp}"
	router                             = google_compute_router.nat_router.name
	region                             = var.region
	nat_ip_allocate_option             = "AUTO_ONLY"
	source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

	subnetwork {
		name                    = google_compute_subnetwork.instance_subnet.self_link
		source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
	}
}

resource "null_resource" "restart_instance" {
	depends_on = [google_compute_instance.default]

	provisioner "local-exec" {
		command = "gcloud config set project ${var.project-id}; gcloud compute instances reset ${google_compute_instance.default.name} --zone=${google_compute_instance.default.zone}"
	}
}

resource "google_project_service" "iap_api" {
	project            = var.project-id
	service            = "iap.googleapis.com"
	disable_on_destroy = false
}

resource "google_iap_tunnel_instance_iam_binding" "rstudio_iap_tunnel" {
	project  = var.project-id
	zone     = var.zone
	instance = google_compute_instance.default.name
	role     = "roles/iap.tunnelResourceAccessor"
	members  = ["user:${var.username}@vindhyadatascience.com"]
}

resource "null_resource" "start_tunnel_script" {
	provisioner "local-exec" {
		command = "chmod +x ./start_tunnel.py && ./start_tunnel.py"
	}
}
