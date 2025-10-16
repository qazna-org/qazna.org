package terraform

deny[msg] {
  some change
  change := input.resource_changes[_]
  change.type == "google_compute_network"
  change.change.after.auto_create_subnetworks
  msg := sprintf("VPC %s must disable auto subnets", [change.name])
}

deny[msg] {
  some change
  change := input.resource_changes[_]
  change.type == "google_compute_subnetwork"
  cidr := change.change.after.ip_cidr_range
  not startswith(cidr, "10.")
  msg := sprintf("Subnet %s must use RFC1918 space", [change.name])
}
