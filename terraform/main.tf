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
  # don't put trailing slashes on paths
  exec_dir            = "/usr/local/passages-signup"
  log_dir             = "/var/log/passages-signup"
  supervisor_conf_dir = "/etc/supervisor/conf.d"

  # non-secret env vars for the program
  enable_lets_encrypt = "true"
  passages_env        = "production"
  public_url          = "https://passages-signup.brandur.org"
}

resource "digitalocean_droplet" "passages_signup" {
  # Get a list of slugs from the API with something like:
  #
  #     curl -X GET --silent "https://api.digitalocean.com/v2/images?per_page=999" -H "Authorization: Bearer $DO_TOKEN" | jq '.images[].slug'
  #
  image = "ubuntu-18-04-x64"

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

  #
  # iptables
  #
  # Note that I started doing this last to give `apt-get` a little more time to
  # become available for use by Terraform. If ran too early while the droplet
  # is still coming up, it has trouble acquiring a lock.
  #
  # A common solution to the problem is to issue a `sleep` command:
  #
  #     https://github.com/hashicorp/terraform/issues/4125
  #

  provisioner "remote-exec" {
    inline = [
      # iptables-persistent has a very questionable interactive install
      # process. These lines work around it by pre-answering the questions it
      # asks. We choose not to save existing IPv4 / IPv6 rules because we'll be
      # configuring everything from scratch anyway.
      "echo iptables-persistent iptables-persistent/autosave_v4 boolean false | sudo debconf-set-selections",
      "echo iptables-persistent iptables-persistent/autosave_v6 boolean false | sudo debconf-set-selections",
      "apt-get -y install iptables-persistent",

      #
      # Note all these rules are inserted in order because they append with `-A
      # input`. It's also possible to insert to a particular position with `-I
      # input <num>` (view position numbers with `iptables -L --line-numbers`),
      # but try to stick to append only to keep the list easy to read.
      #
      # A pretty good article:
      #
      #     https://www.digitalocean.com/community/tutorials/how-to-set-up-a-firewall-using-iptables-on-ubuntu-14-04
      #

      # Accept loopback traffic.
      "iptables -A INPUT -i lo -j ACCEPT",

      # Accept existing connections.
      "iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT",

      # Accept connections for services that we're supposed to be serving.
      "iptables -A INPUT -p tcp --dport 22 -j ACCEPT",
      "iptables -A INPUT -p tcp --dport 80 -j ACCEPT",
      "iptables -A INPUT -p tcp --dport 443 -j ACCEPT",

      # And drop everything else.
      "iptables -A INPUT -j DROP",

      # Persist the rules that we just defined.
      "netfilter-persistent save",
    ]
  }
}

#
# DNS
#

data "digitalocean_domain" "do" {
  name = "do.brandur.org"
}

# A records that allow us to identify specific nodes for easy SSH/etc.
resource "digitalocean_record" "passages_signup_0" {
  domain = "${data.digitalocean_domain.do.name}"
  type   = "A"
  name   = "passages-signup-0"
  ttl    = 600
  value  = "${digitalocean_droplet.passages_signup.ipv4_address}"
}

# An overloaded A/AAAA record for load balancing between nodes.
resource "digitalocean_record" "passages_signup_round_robin_0" {
  domain = "${data.digitalocean_domain.do.name}"
  type   = "A"
  name   = "passages-signup" # value the same for all round robin records
  ttl    = 600
  value  = "${digitalocean_droplet.passages_signup.ipv4_address}"
}

# And the same for IPv6.
resource "digitalocean_record" "passages_signup_ipv6_round_robin_0" {
  domain = "${data.digitalocean_domain.do.name}"
  type   = "AAAA"
  name   = "passages-signup" # value the same for all round robin records
  ttl    = 600
  value  = "${digitalocean_droplet.passages_signup.ipv6_address}"
}

# And a top level CNAME that points back to the round robin A/AAAA record.
resource "cloudflare_record" "passages_signup" {
  domain = "brandur.org"
  name   = "passages-signup"
  value  = "${digitalocean_record.passages_signup_round_robin_0.fqdn}"
  type   = "CNAME"
  ttl    = 1 # magic value 1 sets TTL to "automatic"
}
