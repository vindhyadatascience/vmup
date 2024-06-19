# Launch a GCP instance with Terraform

Terraform is a tool for building, changing, and versioning infrastructure safely and efficiently.

This repository contains the Terraform code to launch a GCP instance.

## Deploying a VM

1. Log into your GCP cloud shell and clone this repository:

    ```bash
    git clone https://github.com/vindhyadatascience/vds-gcp-launch-instance.git
    cd vds-gcp-launch-instance
    ```
2. Initialize Terraform:

    ```bash
    terraform init
    ```

3. (Optional) Create a `terraform.tfvars` file and add the following variables:

    ```hcl
    # terraform.tfvars

    username = "your_username"
    project_id = "your_project_id"
    vm_name = "your_vm_name"
    region = "us-central1-a"
    machine_type = "e2-standard-4"
    image = "https://www.googleapis.com/compute/v1/projects/vds-infrastructure/global/images/vds-debian-12-base"
    ```

    Replace the values with your own.

4. Plan the Terraform configuration:

    ```bash
    terraform plan
    ```

    Review the output and make sure everything looks good.

5. Apply the Terraform configuration:

    ```bash
    terraform apply
    ```

    Confirm the action by typing `yes`.

Once the instance is created, you can SSH into the instance using the IP address printed to the console.

To destroy the instance and all created artifacts, run:

```bash
terraform destroy
```

Confirm the action by typing `yes`.

You can also use the `terraform plan` and  `terraform apply` to update the instance configuration. For example, you can change the machine type by updating the `main.tf` (or `terraform.tfvars`) file and running `terraform plan` then `terraform apply`.

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