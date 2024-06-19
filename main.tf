variable "username" {
  description = "Your GCP username. This is <username>@cloudshell."
}

variable "project_id" {
  description = "Which GCP project ID to create the instance in."
}

variable "vm_name" {
  description = "Name for your VM instance. Use all lowercase letters and no underscores. (e.g. my-awesome-vm)"
}

variable "image" {
  description = "What source image should be used (e.g. vds-debian-12-base, vds-debian-12-rstudio, etc...)"
}

variable "region" {
  default = "us-central1-a"
}

variable "zone" {
  default = "us-central1-a"
}

variable "machine_type" {
  default = "e2-standard-4"
}

provider "google" {
  project = var.project_id
  region = var.region
  zone = var.zone
}

resource "google_compute_instance" "default" {
  name = var.vm_name
  machine_type = var.machine_type
  
  boot_disk {
    auto_delete = true
    device_name = "boot"
    mode = "READ_WRITE"

    initialize_params {
      image = "https://www.googleapis.com/compute/v1/projects/vds-infrastructure/global/images/${var.image}"
      size  = 20
    }
  }

  network_interface {
    network = "default"
    access_config {
      // Ephemeral IP
    }
  }

   metadata = {
    startup-script = "sudo usermod -aG docker ${var.username}"
  }

  tags = ["http-server", "https-server"]
}

resource "google_compute_firewall" "allow-http" {
  name = "allow-http"
  network = "default"

  allow {
    protocol = "tcp"
    ports = ["80", "8787"]
  }
  target_tags = ["http-server"]
  source_ranges = ["0.0.0.0/0"]
}

resource "google_compute_firewall" "allow-https" {
  name = "allow-https"
  network = "default"

  allow {
    protocol = "tcp"
    ports = ["443", "8787"]
  }
  target_tags = ["https-server"]
  source_ranges = ["0.0.0.0/0"]
}

resource "null_resource" "restart_instance" {
  depends_on = [google_compute_instance.default]

  provisioner "local-exec" {
    command = "gcloud config set project ${var.project_id}; gcloud compute instances reset ${google_compute_instance.default.name} --zone=${google_compute_instance.default.zone}"
  }
}

output "instance_ip" {
  value = google_compute_instance.default.network_interface[0].access_config[0].nat_ip
}