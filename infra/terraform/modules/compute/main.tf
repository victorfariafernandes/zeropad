terraform {
  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "~> 6.0"
    }
  }
}

resource "oci_core_instance" "this" {
  compartment_id      = var.compartment_ocid
  availability_domain = var.availability_domain
  display_name        = "zeropad-vm"
  shape               = "VM.Standard.A1.Flex"

  shape_config {
    ocpus         = var.vm_ocpus
    memory_in_gbs = var.vm_memory_gbs
  }

  source_details {
    source_type             = "image"
    source_id               = var.image_id
    boot_volume_size_in_gbs = 50
  }

  create_vnic_details {
    subnet_id        = var.subnet_id
    assign_public_ip = true
    display_name     = "zeropad-vnic"
    hostname_label   = "zeropad"
  }

  metadata = {
    ssh_authorized_keys = "${var.ssh_public_key}\n${var.ssh_deploy_public_key}"
  }

  preserve_boot_volume = false
}

resource "oci_core_instance" "worker" {
  compartment_id      = var.compartment_ocid
  availability_domain = var.availability_domain
  display_name        = "zeropad-worker"
  shape               = "VM.Standard.A1.Flex"

  shape_config {
    ocpus         = var.worker_vm_ocpus
    memory_in_gbs = var.worker_vm_memory_gbs
  }

  source_details {
    source_type             = "image"
    source_id               = var.image_id
    boot_volume_size_in_gbs = 50
  }

  create_vnic_details {
    subnet_id        = var.subnet_id
    assign_public_ip = true
    display_name     = "zeropad-worker-vnic"
    hostname_label   = "zeropad-worker"
  }

  metadata = {
    ssh_authorized_keys = "${var.ssh_public_key}\n${var.ssh_deploy_public_key}"
  }

  preserve_boot_volume = false
}

resource "oci_identity_dynamic_group" "backend" {
  compartment_id = var.tenancy_ocid
  name           = "zeropad-backend"
  description    = "zeropad backend VM for Instance Principal auth"
  matching_rule  = "Any {instance.id = '${oci_core_instance.this.id}', instance.id = '${oci_core_instance.worker.id}'}"
}

resource "oci_core_volume_attachment" "data" {
  attachment_type = "paravirtualized"
  instance_id     = oci_core_instance.this.id
  volume_id       = var.volume_id
  display_name    = "zeropad-data-attachment"
}
