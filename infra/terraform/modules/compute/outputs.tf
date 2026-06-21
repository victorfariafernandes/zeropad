output "vm_public_ip" {
  description = "Public IP of the VM."
  value       = oci_core_instance.this.public_ip
}

output "vm_private_ip" {
  description = "Private IP of the VM (used by the load balancer backend)."
  value       = oci_core_instance.this.private_ip
}

output "backend_dynamic_group_name" {
  description = "Name of the dynamic group used for Instance Principal auth."
  value       = oci_identity_dynamic_group.backend.name
}

output "worker_public_ips" {
  description = "Public IPs of all k3s worker VMs."
  value       = oci_core_instance.worker[*].public_ip
}

output "worker_private_ips" {
  description = "Private IPs of all k3s worker VMs."
  value       = oci_core_instance.worker[*].private_ip
}
