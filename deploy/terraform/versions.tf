terraform {
  required_providers {
    cloudflare = {
      source = "cloudflare/cloudflare"
    }
    digitalocean = {
      source = "terraform-providers/digitalocean"
    }
  }
  required_version = ">= 0.13"
}
