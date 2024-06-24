#!/bin/bash

## Initialize some default variables
timestamp=$(date +"%Y%m%d-%H%M%S")
default_password=$(openssl rand -base64 15)

# Prompt the user for variable values (with default values)
read -p "Enter value for 'username' (default: ${USER}): " username
username=${username:-${USER}}

read -sp "Enter value for 'password' (default: ${default_password}): " password
echo
password=${password:-${default_password}}

read -p "Enter value for 'project_id' (default: eric-sandbox-421120): " project_id
project_id=${project_id:-eric-sandbox-421120}

read -p "Enter value for 'vm-name' (default: instance-${timestamp}): " vm_name
vm_name=${vm_name:-instance-${timestamp}}

read -p "Enter value for 'image' (default: vds-debian-12-rstudio-4-4-1): " image
image=${image:-vds-debian-12-rstudio-4-4-1}

read -p "Enter value for 'region' (default: us-central1): " region
region=${region:-us-central1}

read -p "Enter value for 'zone' (default: us-central1-a): " zone
zone=${zone:-us-central1-a}

read -p "Enter value for 'machine_type' (default: e2-standard-4): " machine_type
machine_type=${machine_type:-e2-standard-4}

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
timestamp="$timestamp"
EOF

# Run Terraform commands
terraform init
terraform apply
