variable "tenancy_ocid" {
  description = "OCID of your OCI tenancy."
  type        = string
}

variable "user_ocid" {
  description = "OCID of the OCI user for API access."
  type        = string
}

variable "fingerprint" {
  description = "Fingerprint of the API signing key."
  type        = string
}

variable "private_key_path" {
  description = "Path to the OCI API private key file (.pem)."
  type        = string
}

variable "region" {
  description = "OCI region (e.g. us-ashburn-1, ap-osaka-1)."
  type        = string
  default     = "us-ashburn-1"
}

variable "compartment_ocid" {
  description = "OCID of the compartment to deploy resources into."
  type        = string
}

variable "ssh_public_key" {
  description = "SSH public key content (not path) to authorize on the VM."
  type        = string
}

variable "ssh_deploy_public_key" {
  description = "SSH public key for the CI/CD deploy pipeline."
  type        = string
}

variable "vm_ocpus" {
  description = "Number of OCPUs for the Ampere A1 shape."
  type        = number
  default     = 2
}

variable "vm_memory_gbs" {
  description = "RAM in GB for the Ampere A1 shape."
  type        = number
  default     = 8
}

variable "worker_vm_ocpus" {
  description = "Number of OCPUs for the k3s worker VM."
  type        = number
  default     = 2
}

variable "worker_vm_memory_gbs" {
  description = "RAM in GB for the k3s worker VM."
  type        = number
  default     = 8
}

variable "worker_count" {
  description = "Number of k3s worker VMs to provision."
  type        = number
  default     = 2
}

variable "block_volume_size_gbs" {
  description = "Block volume size in GB (free up to 200 GB total)."
  type        = number
  default     = 50
}

variable "github_owner" {
  description = "GitHub username or org that owns the ghcr.io package."
  type        = string
}

variable "domain" {
  description = "Apex domain name."
  type        = string
  default     = "zeropad.dev"
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token with DNS edit permission."
  type        = string
  sensitive   = true
}

variable "cloudflare_zone_id" {
  description = "Cloudflare Zone ID for the domain."
  type        = string
}
