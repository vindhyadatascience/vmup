# Launch a GCP instance with Terraform

Terraform is a tool for building, changing, and versioning infrastructure safely and efficiently.

This repository contains the Terraform code to launch a GCP instance.

## Dependencies

Install the following dependencies for your local machine's operating system:

- [Python 3](https://www.python.org/downloads/) - Required for the helper scripts
- [Terraform](https://www.terraform.io/downloads.html) - Infrastructure as Code tool
- [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) - Command line interface for Google Cloud Platform

Install the required Python packages:

```bash
uv pip install google-auth
```

This is a one-time setup per local machine.

## Deploying a VM

1. Open a terminal on your local machine and clone this repository:

   ```bash
   git clone https://github.com/vindhyadatascience/vds-gcp-launch-instance.git
   cd vds-gcp-launch-instance
   ```

2. Launch the setup script and answer the prompts. Press enter to accept defaults.

   ```bash
   ./launch.py
   ```

   This will prompt you for configuration values (project ID, VM name, image, machine type, port mappings, etc.), write them to `terraform.tfvars`, run `terraform init` and `terraform apply`, and start SSH tunnels for the configured port mappings.

Once the instance is created, you can access forwarded services through the SSH tunnels. For images with RStudio, navigate to [localhost:8787](http://localhost:8787/). Your username and password are stored in `~/.env` on the new instance. You can change the password with `sudo passwd {userNameHere}`.

## Managing the instance

### SSH into the instance

To open an interactive SSH session via IAP:

```bash
./ssh.py
```

### Stop tunnels and the instance

To stop the SSH tunnels and optionally stop the instance when you are done working:

```bash
./stop_tunnel.py
```

### Restart tunnels and the instance

To restart the instance and re-establish the SSH tunnels:

```bash
./start_tunnel.py
```

### Destroy the instance

Stop the SSH tunnels before destroying the instance. Then run:

```bash
terraform destroy
```

Confirm the action by typing `yes`.

### Update instance configuration

You can use `terraform plan` and `terraform apply` to update the instance configuration. For example, change the machine type or port mappings by editing `terraform.tfvars` and running `terraform plan` then `terraform apply`.

Port mappings are configured as comma-separated `local:remote` pairs. For example, to forward local port 8787 to remote port 8787 and local port 9000 to remote port 9000:

```
port-mapping="8787:8787,9000:9000"
```

## After deployment

After deploying the instance, you can follow these instructions to customize your environment:

### Adding git credentials

Use the GitHub CLI to authenticate with GitHub:

```bash
gh auth login
```

### Authenticating Docker to GitHub Container Registry.

Create a classic Personal Access Token (PAT) from GitHub (https://github.com/settings/tokens) with the `read:packages` scope selected, save it to a file called `~/.ghcr_token` and authenticate to the `ghcr.io` registry using the following command:

```bash
cat ~/.ghcr_token | docker login ghcr.io -u <username> --password-stdin
```

### Cloning a repository

Clone a repository using the following command:

```bash
git clone <repository>
```
