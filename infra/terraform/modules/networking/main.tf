terraform {
  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "~> 6.0"
    }
  }
}

resource "oci_core_vcn" "this" {
  compartment_id = var.compartment_ocid
  display_name   = "zeropad-vcn"
  cidr_block     = "10.0.0.0/16"
  dns_label      = "zeropad"
}

resource "oci_core_internet_gateway" "this" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.this.id
  display_name   = "zeropad-igw"
  enabled        = true
}

resource "oci_core_route_table" "public" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.this.id
  display_name   = "zeropad-public-rt"

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.this.id
  }
}

resource "oci_core_security_list" "public" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.this.id
  display_name   = "zeropad-public-sl"

  egress_security_rules {
    destination = "0.0.0.0/0"
    protocol    = "all"
  }

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 22
      max = 22
    }
  }

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 80
      max = 80
    }
  }

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 443
      max = 443
    }
  }

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 8080
      max = 8080
    }
  }

  ingress_security_rules {
    protocol = "1"
    source   = "0.0.0.0/0"
    icmp_options {
      type = 3
      code = 4
    }
  }

  # k3s API server
  ingress_security_rules {
    protocol = "6"
    source   = "10.0.0.0/16"
    tcp_options {
      min = 6443
      max = 6443
    }
  }

  # Flannel VXLAN (pod-to-pod overlay)
  ingress_security_rules {
    protocol = "17"
    source   = "10.0.0.0/16"
    udp_options {
      min = 8472
      max = 8472
    }
  }

  # kubelet API
  ingress_security_rules {
    protocol = "6"
    source   = "10.0.0.0/16"
    tcp_options {
      min = 10250
      max = 10250
    }
  }
}

resource "oci_core_subnet" "public" {
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.this.id
  display_name               = "zeropad-public-subnet"
  cidr_block                 = "10.0.0.0/24"
  availability_domain        = var.availability_domain
  route_table_id             = oci_core_route_table.public.id
  security_list_ids          = [oci_core_security_list.public.id]
  dns_label                  = "public"
  prohibit_public_ip_on_vnic = false
}
