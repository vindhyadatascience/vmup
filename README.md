# Launch a GCP instance with Terraform

Terraform is a tool for building, changing, and versioning infrastructure safely and efficiently.

This repository contains the Terraform code to launch a GCP instance.

## Dependencies

Install the following dependencies for your local machine's operating system:

- [Terraform](https://www.terraform.io/downloads.html) - Infrastructure as Code tool
- [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) - Command line interface for Google Cloud Platform

This is a one-time setup per local machine.

## Deploying a VM

1. Open a terminal on your local machine and clone this repository:

   ```bash
   git clone https://github.com/vindhyadatascience/vds-gcp-launch-instance.git
   cd vds-gcp-launch-instance
   ```

2. Launch the wrapper script and answer the prompts. Press enter to accept defaults.

   ```bash
   ./launch.sh
   ```

Once the instance is created, you can SSH into the instance using the ssh tunneling set up by the script. For images with RStudio, you can navigate to [localhost:8787](http://localhost:8787/). Your username and password are stored in ~/.env of the new instance. You can change this password with `sudo passwd {userNameHere}`.

You will need to stop & restart the ssh tunneling as your workflow allows. You can use the following command to stop the ssh tunneling & stop the instance when you are done working.
   ```bash
   ./stop_rstudio_tunnel.sh
   ```

You can restart the tunneling & the instance using the following command.
   ```bash
   ./start_rstudio_tunnel.sh
   ```

You need to stop the ssh tunnleing before you destroy the instance & it's related artifacts.

To destroy the instance and all created artifacts, run:

```bash
terraform destroy
```

Confirm the action by typing `yes`.

You can also use the `terraform plan` and `terraform apply` to update the instance configuration. For example, you can change the machine type by updating the `main.tf` (or `terraform.tfvars`) file and running `terraform plan` then `terraform apply`.

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
