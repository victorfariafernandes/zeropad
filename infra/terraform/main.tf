terraform {
  required_version = ">= 1.6"

  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "~> 6.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }

  # Local state by default.
  # For team use, configure OCI Object Storage as an S3-compatible backend:
  #
  # backend "s3" {
  #   bucket   = "zeropad-tfstate"
  #   key      = "zeropad/terraform.tfstate"
  #   region   = "us-ashburn-1"
  #   endpoint = "https://<namespace>.compat.objectstorage.<region>.oraclecloud.com"
  #   skip_region_validation      = true
  #   skip_credentials_validation = true
  #   skip_metadata_api_check     = true
  #   force_path_style            = true
  # }
}

provider "oci" {
  tenancy_ocid     = var.tenancy_ocid
  user_ocid        = var.user_ocid
  fingerprint      = var.fingerprint
  private_key_path = var.private_key_path
  region           = var.region
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

# ── Data sources ───────────────────────────────────────────────────────────────

data "oci_identity_availability_domains" "ads" {
  compartment_id = var.tenancy_ocid
}

# Ampere A1 capacity is often most available in AD-1.
# Change the index to [1] or [2] if you get "Out of capacity" errors.
locals {
  availability_domain = data.oci_identity_availability_domains.ads.availability_domains[0].name
}

data "oci_core_images" "ol9_arm" {
  compartment_id           = var.compartment_ocid
  operating_system         = "Oracle Linux"
  operating_system_version = "9"
  shape                    = "VM.Standard.A1.Flex"
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
  state                    = "AVAILABLE"
}

data "oci_objectstorage_namespace" "ns" {
  compartment_id = var.compartment_ocid
}

# ── Modules ────────────────────────────────────────────────────────────────────

module "networking" {
  source              = "./modules/networking"
  compartment_ocid    = var.compartment_ocid
  availability_domain = local.availability_domain
}

module "storage" {
  source                   = "./modules/storage"
  compartment_ocid         = var.compartment_ocid
  availability_domain      = local.availability_domain
  block_volume_size_gbs    = var.block_volume_size_gbs
  object_storage_namespace = data.oci_objectstorage_namespace.ns.namespace
}

module "compute" {
  source              = "./modules/compute"
  compartment_ocid    = var.compartment_ocid
  tenancy_ocid        = var.tenancy_ocid
  availability_domain = local.availability_domain
  subnet_id           = module.networking.subnet_id
  volume_id           = module.storage.volume_id
  image_id            = data.oci_core_images.ol9_arm.images[0].id
  ssh_public_key        = var.ssh_public_key
  ssh_deploy_public_key = var.ssh_deploy_public_key
  vm_ocpus             = var.vm_ocpus
  vm_memory_gbs        = var.vm_memory_gbs
  worker_vm_ocpus      = var.worker_vm_ocpus
  worker_vm_memory_gbs = var.worker_vm_memory_gbs
}

resource "oci_identity_policy" "backend_object_storage" {
  compartment_id = var.compartment_ocid
  name           = "zeropad-backend-object-storage"
  description    = "Allow backend VM to manage objects in pads bucket"
  statements = [
    "Allow dynamic-group zeropad-backend to manage objects in compartment id ${var.compartment_ocid} where target.bucket.name = 'zeropad-pads'",
  ]
}

module "dns" {
  source             = "./modules/dns"
  cloudflare_zone_id = var.cloudflare_zone_id
  vm_public_ip       = module.compute.vm_public_ip
  worker_public_ips  = [module.compute.worker_public_ip]
  domain             = var.domain
}
