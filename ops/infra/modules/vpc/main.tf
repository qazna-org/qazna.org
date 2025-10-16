variable "project" {
  description = "GCP project"
  type        = string
}

variable "region" {
  description = "Primary region"
  type        = string
}

provider "google" {
  project = var.project
  region  = var.region
}

resource "google_compute_network" "main" {
  name                    = "qazna-vpc"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "services" {
  name          = "qazna-services"
  ip_cidr_range = "10.20.0.0/20"
  region        = var.region
  network       = google_compute_network.main.id
}
