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

resource "google_project_service" "redis" {
  for_each = toset([
    "redis.googleapis.com",
    "servicenetworking.googleapis.com",
  ])

  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}

resource "google_redis_instance" "primary" {
  project              = var.project_id
  
  authorized_network   = var.redis.authorized_network
  name                 = var.redis.instance_name
  region               = var.redis.region
  tier                 = var.redis.tier
  memory_size_gb       = var.redis.memory_size_gb
  connect_mode         = var.redis.connect_mode
  reserved_ip_range    = var.redis.reserved_ip_range_name

  depends_on = [
    google_project_service.redis["redis.googleapis.com"],
  ]
}

resource "google_project_iam_member" "runner_discovery_job_redis_editor" {
  project = var.project_id
  role    = "roles/redis.editor"
  member  = "serviceAccount:${google_service_account.runner_discovery_job_sa.email}"
}
