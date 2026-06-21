variable "cloudflare_zone_id" {
  description = "Cloudflare Zone ID for the domain."
  type        = string
}

variable "vm_public_ip" {
  description = "Public IP of the VM. Cloudflare A record points here directly."
  type        = string
}

variable "domain" {
  description = "Apex domain name (e.g. zeropad.dev)."
  type        = string
}

variable "worker_public_ips" {
  description = "Public IPs of worker VMs for round-robin DNS."
  type        = list(string)
  default     = []
}
