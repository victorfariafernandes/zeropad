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

# Pads are written under a root-level "folder" (an object name prefix) that
# maps 1:1 to a TTL. All pads currently go under "default/" with a 2-day
# TTL. To offer a different TTL in the future, add another rule here scoped
# to a new prefix (e.g. "long/") and have the backend write to that prefix
# instead — existing prefixes/rules are untouched.
resource "oci_objectstorage_object_lifecycle_policy" "pads" {
  namespace = var.object_storage_namespace
  bucket    = oci_objectstorage_bucket.pads.name

  rules {
    name       = "default-2-day-ttl"
    action     = "DELETE"
    is_enabled = true

    object_name_filter {
      inclusion_prefixes = ["default/"]
    }

    time_amount = "2"
    time_unit   = "DAYS"
  }
}

