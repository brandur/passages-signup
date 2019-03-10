#
# TERRAFORM PROVISIONING SCRIPT
#

# Set in `terraform/terraform.tfvars`.
variable "cloudflare_email" {}
variable "cloudflare_token" {}
variable "database_url" {}
variable "do_token" {}
variable "mailgun_api_key" {}

provider "cloudflare" {
  email = "${var.cloudflare_email}"
  token = "${var.cloudflare_token}"
}

provider "digitalocean" {
  token = "${var.do_token}"
}

#
# SSH
#

data "digitalocean_ssh_key" "brandur" {
  name = "brandur"
}

#
# Droplets
#

locals {
  exec_dir            = "/usr/local/passages-signup/"
  log_dir             = "/var/log/passages-signup/"
  supervisor_conf_dir = "/etc/supervisor/conf.d/"

  # non-secret env vars for the program
  enable_lets_encrypt = "true"
  passages_env        = "production"
  public_url          = "https://passages-signup.do.brandur.org"
}

resource "digitalocean_droplet" "passages_signup" {
  image      = "ubuntu-18-04-x64"
  ipv6       = true
  monitoring = true
  name       = "passages-signup-0"
  region     = "nyc3"
  size       = "s-1vcpu-1gb"
  ssh_keys   = ["${data.digitalocean_ssh_key.brandur.id}"]

  #
  # Go source and dependencies
  #

  provisioner "remote-exec" {
    inline = [
      "mkdir -p ${local.exec_dir}/",
      "mkdir -p ${local.exec_dir}/layouts/",
      "mkdir -p ${local.exec_dir}/public/",
      "mkdir -p ${local.exec_dir}/views/",
    ]
  }

  provisioner "local-exec" {
    command = "GOOS=linux GOARCH=amd64 go build -o passages-signup .."
  }

  provisioner "file" {
    source      = "passages-signup"
    destination = "${local.exec_dir}/passages-signup"
  }

  provisioner "remote-exec" {
    inline = [
      "chmod +x ${local.exec_dir}/passages-signup"
    ]
  }

  provisioner "file" {
    source      = "../layouts" # lack of trailing slash meaningful
    destination = "${local.exec_dir}/"
  }

  provisioner "file" {
    source      = "../public" # lack of trailing slash meaningful
    destination = "${local.exec_dir}/"
  }

  provisioner "file" {
    source      = "../views" # lack of trailing slash meaningful
    destination = "${local.exec_dir}/"
  }

  #
  # Supervisor
  #

  provisioner "remote-exec" {
    inline = [
      "apt-get install -y supervisor",
      "mkdir -p ${local.log_dir}/",
    ]
  }

  provisioner "file" {
    content = <<-EOT
[program:passages-signup]
autostart = true
autorestart = true
command = ${local.exec_dir}/passages-signup
environment = ASSETS_DIR="${local.exec_dir}",DATABASE_URL="${var.database_url}",ENABLE_LETS_ENCRYPT="${local.enable_lets_encrypt}",MAILGUN_API_KEY="${var.mailgun_api_key}",PASSAGES_ENV="${local.passages_env}",PUBLIC_URL="${local.public_url}"
stderr_logfile = ${local.log_dir}/err.log
stdout_logfile = ${local.log_dir}/out.log
    EOT

    destination = "${local.supervisor_conf_dir}/passages-signup.conf"
  }

  provisioner "remote-exec" {
    inline = [
      "supervisorctl reread",
      "supervisorctl update",
    ]
  }
}

#
# DNS
#

data "digitalocean_domain" "do" {
  name = "do.brandur.org"
}

resource "digitalocean_record" "passages_signup_0" {
  domain = "${data.digitalocean_domain.do.name}"
  type   = "A"
  name   = "passages-signup-0"
  value  = "${digitalocean_droplet.passages_signup.ipv4_address}"
}

# Keep the root domain as a `CNAME`, even if it's not a very useful one, so
# that we can more repoint it to something else in the future.
resource "digitalocean_record" "passages_signup" {
  domain = "${data.digitalocean_domain.do.name}"
  name   = "passages-signup"
  type   = "CNAME"
  value  = "${digitalocean_record.passages_signup_0.fqdn}."
}

# And do the same thing to create a direct subdomain for `brandur.org` at
# CloudFlare.
resource "cloudflare_record" "passages_signup" {
  domain = "brandur.org"
  name   = "passages-signup"
  value  = "${digitalocean_record.passages_signup_0.fqdn}"
  type   = "CNAME"
  ttl    = 1 # magic value 1 sets TTL to "automatic"
}
