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

locals {
  job_name = "runner-discovery"
}

resource "google_cloud_run_v2_job" "runner_discovery_job" {
  name     = local.job_name
  location = var.runner_discovery.location

  template {
    template {
      service_account = google_service_account.runner_discovery_job_sa.email
      containers {
        image = var.image
        args  = ["job", local.job_name]
        env {
          name  = "LABEL_QUERY"
          value = join(",", var.runner_discovery.envvars.LABEL_QUERY)
        }
        env {
          name  = "GCP_ORGANIZATION_ID"
          value = var.gcp_organization_id
        }
      }
    }
  }
}

resource "google_service_account" "runner_discovery_job_sa" {
  account_id   = "${local.job_name}-job-sa"
  display_name = "Service Account for ${local.job_name} Cloud Run job"
}

resource "google_project_iam_member" "runner_discovery_job_cloudbuild_viewer" {
  project = var.project_id

  role   = "roles/cloudbuild.viewer"
  member = "serviceAccount:${google_service_account.runner_discovery_job_sa.email}"
}

resource "google_project_iam_member" "runner_discovery_job_project_viewer" {
  project = var.project_id

  role   = "roles/cloudresourcemanager.projectViewer"
  member = "serviceAccount:${google_service_account.runner_discovery_job_sa.email}"
}

resource "google_cloud_scheduler_job" "runner_discovery_scheduler" {
  name             = "${local.job_name}-scheduler"
  schedule         = var.runner_discovery.scheduler_cron
  time_zone        = var.runner_discovery.time_zone
  attempt_deadline = var.runner_discovery.attempt_deadline

  http_target {
    uri         = google_cloud_run_v2_job.runner_discovery_job.uri
    http_method = "POST"
    oauth_token {
      service_account_email = google_service_account.runner_discovery_scheduler_sa.email
    }
  }

  retry_config {
    retry_count = var.runner_discovery.scheduler_retry_limit
  }
}

resource "google_service_account" "runner_discovery_scheduler_sa" {
  account_id   = "${local.job_name}-sch-sa"
  display_name = "Service Account for ${local.job_name} scheduler"
}

resource "google_cloud_run_v2_job_iam_member" "runner_discovery_job_invoker" {
  project = google_cloud_run_v2_job.runner_discovery.project

  location = google_cloud_run_v2_job.runner_discovery.location
  name     = google_cloud_run_v2_job.runner_discovery.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.runner_discovery_scheduler_sa.email}"
}
