# Copyright 2025 The Authors (see AUTHORS file)
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
//
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

resource "google_project_service" "runner_discovery" {
  for_each = toset([
    "cloudasset.googleapis.com",
    "cloudbuild.googleapis.com",
    "cloudscheduler.googleapis.com",
    "run.googleapis.com",
  ])

  project = var.project_id

  service            = each.key
  disable_on_destroy = false
}

resource "google_cloud_run_v2_job" "runner_discovery_job" {
  project = var.project_id

  name     = var.runner_discovery.runner_discovery_job_name
  location = var.runner_discovery.location

  template {
    template {
      service_account = google_service_account.runner_discovery_job_sa.email
      timeout         = "${var.runner_discovery.timeout_seconds}s"

      vpc_access {
        network_interfaces {
          network    = var.vpc_network_name
          subnetwork = var.vpc_subnet_name
        }
        egress = "PRIVATE_RANGES_ONLY"
      }

      containers {
        image = var.image
        args  = ["job", var.runner_discovery.runner_discovery_job_name]
        dynamic "env" {
          for_each = merge(var.runner_discovery.envvars, {
            "REDIS_HOST" = google_redis_instance.primary.host,
            "REDIS_PORT" = google_redis_instance.primary.port,
          })
          content {
            name  = env.key
            value = env.value
          }
        }
      }
    }
  }

  depends_on = [
    google_project_service.runner_discovery,
  ]

  lifecycle {
    ignore_changes = [
      template[0].template[0].containers[0].image,
    ]
  }
}

resource "google_service_account" "runner_discovery_job_sa" {
  project = var.project_id

  account_id   = "${var.runner_discovery.runner_discovery_job_name}-job-sa"
  display_name = "Service Account for ${var.runner_discovery.runner_discovery_job_name} Cloud Run job"
}

resource "google_project_iam_member" "runner_discovery_job_cloudbuild_viewer" {
  project = var.project_id

  role   = "roles/cloudbuild.builds.viewer"
  member = "serviceAccount:${google_service_account.runner_discovery_job_sa.email}"

  depends_on = [
    google_project_service.runner_discovery["cloudbuild.googleapis.com"],
  ]
}

resource "google_folder_iam_member" "runner_discovery_job_project_viewer" {
  folder = var.gcp_folder_id

  role   = var.runner_discovery_custom_role_id
  member = "serviceAccount:${google_service_account.runner_discovery_job_sa.email}"
}

resource "google_cloud_scheduler_job" "runner_discovery_scheduler" {
  project = var.project_id

  name             = "${var.runner_discovery.runner_discovery_job_name}-scheduler"
  schedule         = var.runner_discovery.scheduler_cron
  time_zone        = var.runner_discovery.time_zone
  attempt_deadline = var.runner_discovery.attempt_deadline
  region           = var.runner_discovery.location

  http_target {
    uri         = "https://${var.runner_discovery.location}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${var.project_id}/jobs/${google_cloud_run_v2_job.runner_discovery_job.name}:run"
    http_method = "POST"
    oauth_token {
      service_account_email = google_service_account.runner_discovery_scheduler_sa.email
    }
  }

  retry_config {
    retry_count = var.runner_discovery.scheduler_retry_limit
  }

  depends_on = [
    google_project_service.runner_discovery["cloudscheduler.googleapis.com"],
  ]
}

resource "google_service_account" "runner_discovery_scheduler_sa" {
  project = var.project_id

  account_id   = "${var.runner_discovery.runner_discovery_job_name}-sch-sa"
  display_name = "Service Account for ${var.runner_discovery.runner_discovery_job_name} scheduler"
}

resource "google_cloud_run_v2_job_iam_member" "runner_discovery_job_invoker" {
  project = google_cloud_run_v2_job.runner_discovery_job.project

  location = google_cloud_run_v2_job.runner_discovery_job.location
  name     = google_cloud_run_v2_job.runner_discovery_job.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.runner_discovery_scheduler_sa.email}"
}
