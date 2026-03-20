#!/usr/bin/env python3

import argparse, os, subprocess

parser = argparse.ArgumentParser(description = "")
parser.add_argument("--config_file", type = str, default = "terraform.tfvars")
args = parser.parse_args()

with open(args.config_file) as f:
	config = dict(line.strip().replace('"', '').split("=", 1) for line in f if line.strip())

if os.path.exists(".tunnel_ports"):
	os.remove(".tunnel_ports")

cmd = ["gcloud", "compute", "instances", "start",
	config["vm-name"],
	"--project", config["project-id"],
	"--zone", config["zone"]
]

start_instance = subprocess.run(cmd)

if start_instance.returncode != 0:
	print("Failed to start instance")
else:
	print(f"{config['vm-name']} started successfully")

port_mappings = config["port-mapping"].split(",")

for mapping in port_mappings:
	local, remote = mapping.split(":")
	print(f"Setting up tunnel: local = {local} -> remote = {remote} ...")

	cmd = ["gcloud", "compute", "ssh", config["vm-name"],
		"--project", config["project-id"],
		"--zone", config["zone"],
		"--tunnel-through-iap",
		"--", "-L", f"{local}:localhost:{remote}",
		"-N", "-f"]

	try:
		process = subprocess.Popen(cmd,
			stdin = subprocess.DEVNULL,
			stdout = subprocess.DEVNULL,
			stderr = subprocess.DEVNULL,
			close_fds = True)

		print(f"{local}:{remote} SSH tunnel process started with PID: {process.pid}")

		with open(".tunnel_ports", "a") as f:
			f.write(f"{process.pid} = {local}:{remote}")

	except Exception as e:
		print(f"Failed to start SSH tunnel: {e}")
		print("Please try running ./start_tunnel.py.")
