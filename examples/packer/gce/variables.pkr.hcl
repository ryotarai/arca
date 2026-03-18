variable "project_id" {
  type        = string
  description = "GCP project ID"
}

variable "zone" {
  type        = string
  default     = "us-central1-a"
  description = "GCE zone for building the image"
}

variable "network" {
  type        = string
  default     = "default"
  description = "VPC network"
}

variable "image_family" {
  type        = string
  default     = "arca-gce"
  description = "Image family name for the output image"
}

variable "source_image_family" {
  type        = string
  default     = "ubuntu-2404-lts-amd64"
  description = "Source image family"
}

variable "source_image_project" {
  type        = string
  default     = "ubuntu-os-cloud"
  description = "Source image project"
}
