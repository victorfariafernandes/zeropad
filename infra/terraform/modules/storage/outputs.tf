output "volume_id" {
  description = "OCID of the block volume."
  value       = oci_core_volume.data.id
}

output "bucket_name" {
  description = "Name of the Object Storage pads bucket."
  value       = oci_objectstorage_bucket.pads.name
}

output "postgres_backups_bucket_name" {
  description = "Name of the OCI bucket used for PostgreSQL WAL archiving and base backups."
  value       = oci_objectstorage_bucket.postgres_backups.name
}
