terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }

  backend "gcs" {
    bucket = "qazna-terraform-state"
    prefix = "dev"
  }
}

data "google_client_config" "current" {}

module "network" {
  source  = "../../modules/vpc"
  project = var.project_id
  region  = var.region
}
