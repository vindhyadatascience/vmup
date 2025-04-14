variable "username" {
  description = "Your GCP username. This is <username>@cloudshell."
}

variable "password" {
  description = "Configure your user account password."
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
  default = "us-central1"
}

variable "zone" {
  default = "us-central1-a"
}

variable "machine_type" {
  default = "e2-standard-4"
}

variable "timestamp" {
  description = "Timestamp for assigning instance id and static address name."
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

resource "google_compute_instance" "default" {
  name         = var.vm_name
  machine_type = var.machine_type

  boot_disk {
    auto_delete = true
    device_name = "boot"
    mode        = "READ_WRITE"

    initialize_params {
      image = "https://www.googleapis.com/compute/v1/projects/vds-infrastructure/global/images/${var.image}"
      size  = 20
    }
  }

  network_interface {
    network = "default"
  }

  metadata = {
    startup-script = <<-EOF
    #!/bin/bash
    # Add user to docker group
    sudo usermod -aG docker ${var.username}

    # Generate temporary password for user
    echo "Username=${var.username}" > /home/${var.username}/.env
    echo "Password=${var.password}" >> /home/${var.username}/.env
    echo "${var.username}:${var.password}" | sudo chpasswd
    EOF
  }

  tags = ["http-server", "https-server"]
}

resource "google_compute_firewall" "allow-http" {
  name    = "allow-http-${var.timestamp}"
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["80", "8787"]
  }
  target_tags   = ["http-server"]
  source_ranges = ["35.235.240.0/20"]
}

resource "google_compute_firewall" "allow-https" {
  name    = "allow-https-${var.timestamp}"
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["443", "8787"]
  }
  target_tags   = ["https-server"]
  source_ranges = ["35.235.240.0/20"]
}

resource "google_compute_firewall" "allow-ssh-ingress-from-iap" {
  name      = "allow-ssh-ingress-from-iap-${var.timestamp}"
  network   = "default"
  direction = "INGRESS"
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
  source_ranges = ["35.235.240.0/20"]
}

# Create a router with timestamp in the name
resource "google_compute_router" "nat_router" {
  name    = "nat-router-${var.timestamp}"
  network = "default"
  region  = var.region
}

# Create NAT configuration with timestamp in the name
resource "google_compute_router_nat" "nat_config" {
  name                               = "nat-config-${var.timestamp}"
  router                             = google_compute_router.nat_router.name
  region                             = var.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

resource "null_resource" "restart_instance" {
  depends_on = [google_compute_instance.default]

  provisioner "local-exec" {
    command = "gcloud config set project ${var.project_id}; gcloud compute instances reset ${google_compute_instance.default.name} --zone=${google_compute_instance.default.zone}"
  }
}

# Enable required IAP services
resource "google_project_service" "iap_api" {
  project            = var.project_id
  service            = "iap.googleapis.com"
  disable_on_destroy = false
}

# Add IAP tunnel access to your instance
resource "google_iap_tunnel_instance_iam_binding" "rstudio_iap_tunnel" {
  project  = var.project_id
  zone     = var.zone
  instance = google_compute_instance.default.name
  role     = "roles/iap.tunnelResourceAccessor"
  members  = ["user:${var.username}@vindhyadatascience.com"]
}

# Create the SSH tunnel control scripts
resource "local_file" "start_tunnel_script" {
  filename = "start_rstudio_tunnel.sh"
  content  = <<-EOT
    #!/bin/bash
    
    # Kill any existing tunnels to prevent port conflicts
    pkill -f "ssh.*8787" 2>/dev/null

    # Start the VM instance (if stopped)
    gcloud compute instances start ${google_compute_instance.default.name} \
      --project=${var.project_id} \
      --zone=${var.zone}
    
    # Start the tunnel in the background
    gcloud compute ssh ${google_compute_instance.default.name} \
      --project=${var.project_id} \
      --zone=${var.zone} \
      --tunnel-through-iap \
      -- -L 8787:localhost:8787 -N -f
    
    tunnel_status=$?
    
    if [ $tunnel_status -eq 0 ]; then
      echo "RStudio tunnel established successfully!"
      echo "Access RStudio at http://localhost:8787"
      echo "Username: ${var.username}"
      echo "Password: As configured during setup"
      echo "To stop the tunnel and VM, run: ./stop_rstudio_tunnel.sh"
    else
      echo "Failed to establish tunnel. Please try again."
    fi
    
    # Explicitly exit the script
    exit $tunnel_status
  EOT

  provisioner "local-exec" {
    command = "chmod +x start_rstudio_tunnel.sh"
  }
}

resource "local_file" "stop_tunnel_script" {
  filename = "stop_rstudio_tunnel.sh"
  content  = <<-EOT
    #!/bin/bash
    
    # Kill any existing tunnels
    echo "Closing SSH tunnel..."
    pkill -f "ssh.*8787"
    
    # Optionally stop the VM to save costs
    read -p "Do you want to stop the VM (${google_compute_instance.default.name}) to save costs? (y/N): " STOP_VM
    if [[ "$STOP_VM" =~ ^[Yy]$ ]]; then
      echo "Stopping VM ${google_compute_instance.default.name}..."
      gcloud compute instances stop ${google_compute_instance.default.name} \
        --project=${var.project_id} \
        --zone=${var.zone}
      
      echo "VM stopped successfully."
      echo "To start the VM and re-establish the tunnel, run:"
      echo "Then run: ./start_rstudio_tunnel.sh"
    else
      echo "VM is still running. Tunnel closed."
      echo "To restart the tunnel, run: ./start_rstudio_tunnel.sh"
    fi
  EOT

  provisioner "local-exec" {
    command = "chmod +x stop_rstudio_tunnel.sh"
  }
}

output "username" {
  value = var.username
}

output "password" {
  value     = var.password
  sensitive = true
}

output "rstudio_url" {
  value       = "http://localhost:8787"
  description = "Access RStudio at this URL after the tunnel is established"
}

output "tunnel_commands" {
  value       = <<-EOT
    Start tunnel: ./start_rstudio_tunnel.sh
    Stop tunnel:  ./stop_rstudio_tunnel.sh
  EOT
  description = "Commands to manage the RStudio SSH tunnel"
}
