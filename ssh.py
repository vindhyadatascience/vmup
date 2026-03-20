#!/usr/bin/env python3

import argparse, subprocess

parser = argparse.ArgumentParser(description = "SSH into the instance via IAP")
parser.add_argument("--config_file", type = str, default = "terraform.tfvars")
args = parser.parse_args()

with open(args.config_file) as f:
	config = dict(line.strip().replace('"', '').split("=", 1) for line in f if line.strip())

cmd = ["gcloud", "compute", "ssh", config["vm-name"],
	"--project", config["project-id"],
	"--zone", config["zone"],
	"--tunnel-through-iap"]

subprocess.run(cmd)
