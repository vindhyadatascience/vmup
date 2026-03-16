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
  default = "e2-highmem-2"
}

variable "boot_disk_size" {
  default = 20
  type = number
}

variable "timestamp" {
  description = "Timestamp for assigning instance id and static address name."
}

variable "server_ports" {
  description = "List of server ports to forward (comma-separated string, first port should be RStudio)"
  default     = ""
  type        = string
}

variable "number_of_ports" {
  description = "Number of SSH tunnel ports to configure"
  default     = 1
  type        = number
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
      size  = var.boot_disk_size
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
    sudo apt-get update && sudo apt-get dist-upgrade

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
    # Use a broader port range to accommodate any dynamically assigned port
    ports    = ["80", "8000-9999"]
  }
  target_tags   = ["http-server"]
  source_ranges = ["35.235.240.0/20"]
}

resource "google_compute_firewall" "allow-https" {
  name    = "allow-https-${var.timestamp}"
  network = "default"

  allow {
    protocol = "tcp"
    # Use a broader port range to accommodate any dynamically assigned port
    ports    = ["443", "8000-9999"]
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
  content  = <<EOT
#!/bin/bash

# Function to find the first available port starting from a given port
find_available_port() {
    local port=$1
    local max_port=$((port + 1000))  # Limit the search to port+1000

    while [ $port -le $max_port ]; do
        # Check if the port is in use - more thorough check
        if ! (netstat -tuln | grep -q ":$port " || lsof -i:$port > /dev/null 2>&1); then
            echo $port
            return 0
        fi
        port=$((port + 1))
    done

    # If we get here, no ports were available
    echo "No available ports found between $1 and $max_port"
    return 1
}

# Get the number of tunnel ports to set up
# NUM_PORTS=${var.number_of_ports}

# Find available ports for all services

IFS=',' read -ra SERVER_PORTS <<< "${var.server_ports}"

# SERVER_PORTS=split(",", var.server_ports)

# Clean up any previous port tracking
rm -f .tunnel_ports

# Start the VM instance (if stopped)
gcloud compute instances start ${google_compute_instance.default.name} \
  --project=${var.project_id} \
  --zone=${var.zone}

# If this isn't here, the ssh tunneling fails the first time.
sleep 20

# Kill any existing tunnels (cleanup before starting new ones)
pkill -f "ssh.*-L.*localhost" 2>/dev/null

echo "=============================="
echo " SETTING UP SSH TUNNELS"
echo "=============================="

success_count=0

# Set up tunnels for all found ports
for ((i=0; i<$${#SERVER_PORTS[@]}; i++)); do

    if [ $i -eq 0 ]; then
      SERVER_SIDE_PORT=8787
    else
      SERVER_SIDE_PORT=$${SERVER_PORTS[$i]}
    fi

    LOCAL_PORT=$${SERVER_PORTS[$i]}
    echo "Setting up tunnel $((i+1)) of $${#SERVER_PORTS[@]} using local port $LOCAL_PORT..."

    # Store the mapping in the ports tracking file
    echo "$LOCAL_PORT:$${SERVER_PORTS[$i]}:$([ $i -eq 0 ] && echo 'RStudio' || echo 'Service')" >> .tunnel_ports

    # Kill any existing tunnels to prevent port conflicts
    pkill -f "ssh.*:$LOCAL_PORT" 2>/dev/null

    # Start the tunnel in the background
    gcloud compute ssh ${google_compute_instance.default.name} \
      --project=${var.project_id} \
      --zone=${var.zone} \
      --tunnel-through-iap \
      -- -L $LOCAL_PORT:localhost:$SERVER_SIDE_PORT -N -f

    tunnel_status=$?

    if [ $tunnel_status -eq 0 ]; then
      success_count=$((success_count + 1))
    else
      echo "Failed to establish tunnel $((i+1)). Please try again."
    fi

    echo "------------------------------"
done

if [ $success_count -gt 0 ]; then
  echo ""
  echo "=============================="
  echo " SETUP COMPLETE"
  echo "=============================="
  echo "Your tunnels are ready!"
  echo ""

  # Display all the port mappings
  echo "Port mappings:"
  cat .tunnel_ports | while read mapping; do
    IFS=':' read -r local_port server_port service <<< "$mapping"
    echo "localhost:$local_port -> server:$server_port ($service)"

    # If this is the RStudio port (first one), provide additional info
    echo "URL: http://localhost:$local_port"
  done

  echo ""
  echo "To manage the SSH tunnels:"
  echo "- Start: ./start_rstudio_tunnel.sh"
  echo "- Stop:  ./stop_rstudio_tunnel.sh"
  echo ""
  echo "If your connection is lost, simply run ./start_rstudio_tunnel.sh to reconnect."
  echo "=============================="
else
  echo "All tunnel setup attempts failed. Please check your network and try again."
fi

# Explicitly exit the script
exit $([ $success_count -gt 0 ] && echo 0 || echo 1)
EOT

  provisioner "local-exec" {
    command = "chmod +x start_rstudio_tunnel.sh"
  }
}

# Stop script
resource "local_file" "stop_tunnel_script" {
  filename = "stop_rstudio_tunnel.sh"
  content  = <<-EOT
#!/bin/bash

# Get the ports from the file if it exists
if [ -f .tunnel_ports ]; then
    echo "Closing SSH tunnels..."
    cat .tunnel_ports | while read mapping; do
        IFS=':' read -r local_port server_port service <<< "$$mapping"
        echo "Closing SSH tunnel on port $$local_port ($$service)..."
        pkill -f "ssh.*$$local_port" 2>/dev/null
    done
    # Remove the ports file
    rm .tunnel_ports
    # Also remove the RStudio port file if it exists
    [ -f .rstudio_port ] && rm .rstudio_port
else
    # For backward compatibility, check for old ports file
    if [ -f .rstudio_ports ]; then
        echo "Closing SSH tunnels..."
        cat .rstudio_ports | while read local_port; do
            echo "Closing SSH tunnel on port $$local_port..."
            pkill -f "ssh.*$$local_port" 2>/dev/null
        done
        rm .rstudio_ports
    else
        # Default to killing all SSH tunnels if no port file exists
        echo "Closing all SSH tunnels..."
        pkill -f "ssh.*-L.*localhost" 2>/dev/null
    fi
fi

# Optionally stop the VM to save costs
read -p "Do you want to stop the VM (${google_compute_instance.default.name}) to save costs? (y/N): " STOP_VM
if [[ "$STOP_VM" =~ ^[Yy]([Ee][Ss])?$ ]]; then
  echo "Stopping VM ${google_compute_instance.default.name}..."
  gcloud compute instances stop ${google_compute_instance.default.name} \
    --project=${var.project_id} \
    --zone=${var.zone}

  echo "VM stopped successfully."
  echo "To start the VM and re-establish the tunnels, run: ./start_rstudio_tunnel.sh"
else
  echo "VM is still running. Tunnels closed."
  echo "To restart the tunnels, run: ./start_rstudio_tunnel.sh"
fi
EOT

  provisioner "local-exec" {
    command = "chmod +x stop_rstudio_tunnel.sh"
  }
}
