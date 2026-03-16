#!/bin/bash

## Initialize some default variables
timestamp=$(date +"%Y%m%d-%H%M%S")
default_password=$(openssl rand -hex 15)
default_project=$(gcloud config list --format 'value(core.project)')

# Prompt the user for variable values (with default values)
# read -p "Enter value for 'username' (default: ${USER}): " username
username=${username:-${USER}}

# This approach generates a password at runtime for Rstudio so this is not needed.
# read -sp "Enter value for 'password' (default: ${default_password}): " password
# echo
password=${password:-${default_password}}

if [ ! -z "$default_project" ]
then
    read -p "Enter value for 'project_id' (default: ${default_project}): " project_id
    project_id=${project_id:-${default_project}}
else
    while [[ -z "$project_id" ]]
    do
        read -p "Enter value for 'project_id': " project_id
    done
fi

read -p "Enter value for 'vm-name' (default: instance-${timestamp}): " vm_name
vm_name=${vm_name:-instance-${timestamp}}

read -p "Enter value for 'image' (default: vds-debian-12-base): " image
image=${image:-vds-debian-12-base}

read -p "Enter value for 'region' (default: us-central1): " region
region=${region:-us-central1}

read -p "Enter value for 'zone' (default: us-central1-a): " zone
zone=${zone:-us-central1-a}

read -p "Enter value for 'machine_type' (default: e2-highmem-2): " machine_type
machine_type=${machine_type:-e2-highmem-2}

read -p "Enter value for 'boot_disk_size' (default: 20GB): " boot_disk_size
boot_disk_size=${boot_disk_size:-20}

read -p "Enter comma-separated list of ports to forward (default: 8787): " server_ports
server_ports=${server_ports:-8787}

read -p "Enter value for 'number_of_ports' (default: 1): " number_of_ports
number_of_ports=${number_of_ports:-1}

# Write the values to a .tfvars file
cat <<EOF > terraform.tfvars
username = "$username"
password = "$password"
project_id = "$project_id"
vm_name = "$vm_name"
image = "$image"
region = "$region"
zone = "$zone"
machine_type = "$machine_type"
boot_disk_size = "$boot_disk_size"
timestamp="$timestamp"
server_ports="$server_ports"
number_of_ports="$number_of_ports"
EOF

# Run Terraform commands and check for errors
terraform init
if [ $? -ne 0 ]; then
  echo "Terraform initialization failed. Please check the errors above."
  exit 1
fi

terraform apply
if [ $? -ne 0 ]; then
  echo "Terraform apply failed. Please check the errors above."
  exit 1
fi

# # Only display completion message if all commands succeeded
# echo ""
# echo "=============================="
# echo " SETUP COMPLETE"
# echo "=============================="
# echo ""
# echo "Exposed Port: http://localhost:$PORT"
# echo ""
# echo "To manage the SSH tunnel:"
# echo "- Start: ./start_rstudio_tunnel.sh"
# echo "- Stop:  ./stop_rstudio_tunnel.sh"
# echo ""
# echo "If your connection is lost, simply run ./start_rstudio_tunnel.sh to reconnect."
# echo "=============================="
# echo "Starting tunnel..."
./start_rstudio_tunnel.sh
