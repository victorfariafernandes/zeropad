variable "compartment_ocid" {
  description = "OCI compartment OCID."
  type        = string
}

variable "tenancy_ocid" {
  description = "OCI tenancy OCID (required for dynamic group at tenancy root)."
  type        = string
}

variable "availability_domain" {
  description = "OCI availability domain name."
  type        = string
}

variable "subnet_id" {
  description = "OCID of the subnet to attach the VM to."
  type        = string
}

variable "volume_id" {
  description = "OCID of the block volume to attach."
  type        = string
}

variable "image_id" {
  description = "OCID of the Oracle Linux 9 ARM image."
  type        = string
}

variable "ssh_public_key" {
  description = "SSH public key content to authorize on the VM."
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
  description = "Number of OCPUs for the k3s worker Ampere A1 VM."
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
