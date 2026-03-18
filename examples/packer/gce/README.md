# Arca GCE Machine Image (Packer)

Packer template for building a pre-provisioned GCE machine image for Arca. Using a custom image significantly reduces machine startup time because system packages and user configuration are already in place.

## Prerequisites

- [Packer](https://developer.hashicorp.com/packer/install) >= 1.9
- GCP credentials configured (`gcloud auth application-default login` or a service account)
- A GCP project with the Compute Engine API enabled

## Usage

```bash
# Initialize Packer plugins
packer init .

# Build the image (replace YOUR_PROJECT_ID)
packer build -var 'project_id=YOUR_PROJECT_ID' .
```

## Variables

| Variable | Default | Description |
|---|---|---|
| `project_id` | *(required)* | GCP project ID |
| `zone` | `us-central1-a` | GCE zone for the build instance |
| `network` | `default` | VPC network for the build instance |
| `image_family` | `arca-gce` | Image family name for the output image |
| `source_image_family` | `ubuntu-2404-lts-amd64` | Source image family |
| `source_image_project` | `ubuntu-os-cloud` | Source image project |

Override defaults with `-var` flags or a `.pkrvars.hcl` file:

```bash
packer build -var-file=my.pkrvars.hcl .
```

## What's included in the image

- System packages: bash, ca-certificates, curl, git, jq, python3, tmux, ttyd, build-essential, sudo, ansible
- `arca` system group, `arcad` daemon user, `arcauser` interactive user
- Required directories: `/workspace`, `/etc/arca`, `/opt/arca`, `/var/lib/arca`
- Sudoers configuration for `arcauser`

## What's NOT included

- **arcad binary**: Downloaded via cloud-init on every boot to ensure machines always run the latest version.

## Registering the image in Arca

After the image is built, register it as a Custom Image in the Arca UI:

1. Open the Arca web UI and navigate to the runtime settings page.
2. Add a new Custom Image entry with the GCE image name or family (e.g., `arca-gce`).
3. Create machines using the custom image. They will start faster because provisioning steps are already applied.

Since arcad's Ansible setup is idempotent, pre-provisioned images work seamlessly — already-satisfied steps are skipped on boot.
