#!/usr/bin/env python3

import os, json, subprocess, getpass, pprint
from datetime import datetime
import google.auth

TIMESTAMP = datetime.now().strftime("%Y%m%d-%H%M%S")
DEFAULT_PASSWORD = subprocess.check_output(["openssl", "rand", "-hex", "15"]).strip().decode("utf-8")
_, DEFAULT_PROJECT = google.auth.default()
DEFAULT_USER = getpass.getuser()

def capture_input(dict):

	tfvars = dict

	tfvars["username"] = input(f"Enter your GCP username (part before @ in your email address) (default: {DEFAULT_USER}): ") or tfvars.get("username", DEFAULT_USER)
	tfvars["password"] = DEFAULT_PASSWORD
	tfvars["timestamp"] = TIMESTAMP

	tfvars["project-id"] = input(f"Enter value for 'project-id' (default: {DEFAULT_PROJECT}): ") or tfvars.get("project-id", DEFAULT_PROJECT)

	tfvars["vm-name"] = input(f"Enter value for 'vm-name (default: 'instance-{TIMESTAMP}'): ") or tfvars.get("vm-name", f"instance-{TIMESTAMP}")

	tfvars["image"] = input(f"Enter value for 'image' (options: [vds-debian-13-base, vds-debian-13-rstudio-4-5-3, vds-ubuntu-2404-lts-amd64-base, vds-ubuntu-2404-lts-amd64-rstudio-4-5-3]): ") or tfvars.get("image", "vds-debian-13-base")

	tfvars["region"] = input(f"Enter value for 'region' (default: 'us-central1'): ") or tfvars.get("region", "us-central1")

	tfvars["zone"] = input(f"Enter value for 'zone' (default: 'us-central1-a'): ") or tfvars.get("zone", "us-central1-a")

	tfvars["machine-type"] = input(f"Enter value for 'machine-type' (default: 'e2-highmem-2'): ") or tfvars.get("machine_type", "e2-highmem-2")

	tfvars["boot-disk-size"] = input(f"Enter value for 'boot-disk-size' in gigabytes (default: '20'): ") or tfvars.get("boot-disk-size", "20")

	tfvars["port-mapping"] = input(f"Enter comma-separated port mapping as local:remote (default: '8787:8787', e.g. '8787:8787,2222:22'): ") or tfvars.get("port-mapping", "8787:8787")

	return tfvars

tfvars = {}
qedit = "y"

while qedit == "y":
	tfvars = capture_input(tfvars)
	print("Selected values: ")
	pprint.pprint(tfvars, indent = 2)

	qedit = input("Would you like to change any of these values? [y/N]: ") or "N"

with open("terraform.tfvars", "w") as f:
	for key, value in tfvars.items():
		f.write(f'{key}="{value}"\n')

# subprocess.run(["terraform", "init"])
# subprocess.run(["terraform", "apply"])
# subprocess.run(["./start_rstudio_tunnel.sh"])
