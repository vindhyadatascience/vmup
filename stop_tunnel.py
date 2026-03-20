#!/usr/bin/env python3

import argparse, os

parser = argparse.ArgumentParser(description = "")
parser.add_argument("--config_file", type = str, default = "terraform.tfvars")
args = parser.parse_args()

with open(args.config_file) as f:
	config = dict(line.strip().split("=", 1) for line in f if line.strip())

with open(".tunnel_ports", "r") as f:
	port_pids = dict(line.strip().split("=", 1) for line in f if line.strip())

for pid, ports in port_pids.items():
	print(f"Terminating PID {pid} controlling tunnel from {ports}")
	subprocess.run(["kill", pid])

os.remove(".tunnel_ports")

print("All SSH tunnels for this project have been closed.")

stop_vm = input("Would you like to stop the VM to save costs? [y/N]: ") or "N"

if stop_vm is "y":

	cmd = ["gcloud", "compute", "instances", "stop",
		config["vm-name"],
		"--project", config["project-id"],
		"--zone", config["zone"]
	]

	stop_instance = subprocess.run(cmd)

	if stop_instance.returncode != 0:
		print("Failed to stop instance")
	else:
		print(f"{config['vm-name']} stopped successfully")
		print( "To start the VM and re-establish the tunnels, run: ./start_tunnel.py")

print("To completely remove the VM, run 'terraform destroy'")
