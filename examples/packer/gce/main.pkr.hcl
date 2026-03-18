packer {
  required_plugins {
    googlecompute = {
      version = ">= 1.1.0"
      source  = "github.com/hashicorp/googlecompute"
    }
  }
}

source "googlecompute" "arca" {
  project_id          = var.project_id
  zone                = var.zone
  network             = var.network
  source_image_family = var.source_image_family
  source_image_project_id = [var.source_image_project]
  machine_type        = "e2-standard-2"
  disk_size           = 40
  image_name          = "arca-gce-{{timestamp}}"
  image_family        = var.image_family
  image_description   = "Arca machine image with pre-installed system packages"
  ssh_username        = "packer"
}

build {
  sources = ["source.googlecompute.arca"]

  provisioner "shell" {
    script          = "scripts/provision.sh"
    execute_command = "sudo sh -c '{{ .Path }}'"
  }
}
