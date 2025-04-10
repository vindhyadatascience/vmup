#!/bin/bash

## Initialize some default variables
timestamp=$(date +"%Y%m%d-%H%M%S")
default_password=$(openssl rand -base64 15)
declare -a default_project_id=(
    [eric]=eric-sandbox-421120 \
    [adnan]=sandbox-ad-359715 \
    [sarah]=sad-sandbox \
    [alex]=rstudio-server-instance \
    [chinmaya]=chinmaya-sandbox \
    [jared]=jared-project-378819 \
)

# Prompt the user for variable values (with default values)
read -p "Enter value for 'username' (default: ${USER}): " username
username=${username:-${USER}}

read -sp "Enter value for 'password' (default: ${default_password}): " password
echo
password=${password:-${default_password}}

if [ ! -z "${default_project_id[$username]}" ]
then
    read -p "Enter value for 'project_id' (default: ${default_project_id[$username]}): " project_id
    project_id=${project_id:-${default_project_id[$username]}}
else
    while [[ -z "$project_id" ]]
    do
        read -p "Enter value for 'project_id': " project_id
    done
fi

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

echo ""
echo "=============================="
echo " SETUP COMPLETE"
echo "=============================="
echo "Your RStudio environment is ready!"
echo ""
echo "RStudio URL: http://localhost:8787"
echo "Username: $username"
echo "Password: $password"
echo ""
echo "To manage the SSH tunnel:"
echo "- Start: ./start_rstudio_tunnel.sh"
echo "- Stop:  ./stop_rstudio_tunnel.sh"
echo ""
echo "If your connection is lost, simply run ./start_rstudio_tunnel.sh to reconnect."
echo "=============================="
echo "Starting RStudio tunnel..."
./start_rstudio_tunnel.sh