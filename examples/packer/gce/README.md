# Arca GCE Machine Image (Packer)

Packer template for building a pre-provisioned GCE machine image for Arca. The build downloads `arcad` from a running Arca server and runs `arcad --setup-once`, which executes the embedded Ansible playbook to install system packages and configure the environment. Using a custom image significantly reduces machine startup time because provisioning steps are already applied.

## Prerequisites

- [Packer](https://developer.hashicorp.com/packer/install) >= 1.9
- GCP credentials configured (`gcloud auth application-default login` or a service account)
- A GCP project with the Compute Engine API enabled
- A running Arca server with an API token configured (`ARCA_API_TOKEN`)

## Usage

```bash
# Initialize Packer plugins
packer init .

# Build the image
packer build \
  -var 'project_id=YOUR_PROJECT_ID' \
  -var 'arca_server_url=http://arca.example.com:8080' \
  -var 'arca_api_token=YOUR_API_TOKEN' \
  .
```

## Variables

| Variable | Default | Description |
|---|---|---|
| `project_id` | *(required)* | GCP project ID |
| `arca_server_url` | *(required)* | Base URL of the Arca server |
| `arca_api_token` | *(required)* | API token for authenticating with the Arca server |
| `zone` | `us-central1-a` | GCE zone for the build instance |
| `network` | `default` | VPC network for the build instance |
| `image_family` | `arca-gce` | Image family name for the output image |
| `source_image_family` | `ubuntu-2404-lts-amd64` | Source image family |
| `source_image_project` | `ubuntu-os-cloud` | Source image project |

Override defaults with `-var` flags or a `.pkrvars.hcl` file:

```bash
packer build -var-file=my.pkrvars.hcl .
```

## How it works

1. The Packer build downloads the `arcad` binary from the Arca server.
2. `arcad --setup-once` runs the embedded Ansible playbook in offline mode, installing system packages, creating users, and configuring directories.
3. The `arcad` binary is removed after setup — it is downloaded fresh via cloud-init on every boot so machines always run the latest version.

Since arcad's setup is idempotent, pre-provisioned images work seamlessly — already-satisfied steps are skipped on boot.

## Registering the image in Arca

After the image is built, register it as a Custom Image in the Arca UI:

1. Open the Arca web UI and navigate to the runtime settings page.
2. Add a new Custom Image entry with the GCE image name or family (e.g., `arca-gce`).
3. Create machines using the custom image. They will start faster because provisioning steps are already applied.
