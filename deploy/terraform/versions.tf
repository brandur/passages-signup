terraform {
  required_providers {
    cloudflare = {
      source = "terraform-providers/cloudflare"
    }
    digitalocean = {
      source = "terraform-providers/digitalocean"
    }
  }
  required_version = ">= 0.13"
}
