# Copyright 2025 The Authors (see AUTHORS file)
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

resource "google_project_service" "runner_registry" {
  for_each = toset([
    "redis.googleapis.com",
    "servicenetworking.googleapis.com",
  ])

  project = var.project_id

  service            = each.key
  disable_on_destroy = false
}

data "google_compute_network" "shared_network" {
  project = var.vpc_host_project_id

  name = var.vpc_network_name
}

data "google_compute_subnetwork" "shared_subnetwork" {
  project = var.vpc_host_project_id

  name   = var.vpc_subnet_name
  region = var.vpc_subnet_region
}

resource "google_redis_instance" "primary" {
  project = var.project_id

  authorized_network = data.google_compute_network.shared_network.id
  name               = var.runner_registry.instance_name
  region             = var.runner_registry.region
  tier               = var.runner_registry.tier
  memory_size_gb     = var.runner_registry.memory_size_gb
  connect_mode       = var.runner_registry.connect_mode
  reserved_ip_range  = var.runner_registry.reserved_ip_range_name

  depends_on = [
    google_project_service.runner_registry["redis.googleapis.com"],
  ]
}

resource "google_project_iam_member" "runner_discovery_job_registry_editor" {
  project = var.project_id

  role   = "roles/redis.editor"
  member = "serviceAccount:${google_service_account.runner_discovery_job_sa.email}"
}
