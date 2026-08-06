terraform {
  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "~> 6.0"
    }
  }
}

resource "oci_core_volume" "data" {
  compartment_id      = var.compartment_ocid
  availability_domain = var.availability_domain
  display_name        = "zeropad-data"
  size_in_gbs         = var.block_volume_size_gbs
  vpus_per_gb         = 10
}

resource "oci_objectstorage_bucket" "pads" {
  compartment_id = var.compartment_ocid
  namespace      = var.object_storage_namespace
  name           = "zeropad-pads"
  access_type    = "NoPublicAccess"
  versioning     = "Disabled"
}

