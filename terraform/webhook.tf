# Copyright 2023 The Authors (see AUTHORS file)
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

locals {
  key_mount_path = "/etc/secrets/webhook/key"
  key_name       = "webhook-key"
}

resource "google_project_service" "default" {
  for_each = toset([
    "cloudbuild.googleapis.com",
    "cloudkms.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "compute.googleapis.com", # Needed to give the automation bot serviceAccountUser permission over the default compute SA 
    "logging.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "serviceusage.googleapis.com",
    "sts.googleapis.com",
    "secretmanager.googleapis.com",
  ])

  project = var.project_id

  service                    = each.value
  disable_on_destroy         = false
  disable_dependent_services = false # To keep, or not to keep? From github-wif module
}

resource "google_service_account" "run_service_account" {
  project = var.project_id

  account_id   = "gad-webhook-sa"
  display_name = "gad-webhook-sa Cloud Run Service Account"
}

module "gclb" {
  count = var.enable_gclb ? 1 : 0

  source = "git::https://github.com/abcxyz/terraform-modules.git//modules/gclb_cloud_run_backend?ref=ebaccaa0c906e89813e3b0b71fc5fc6be9ef0cdb"

  project_id = var.project_id

  name             = var.name
  run_service_name = module.cloud_run.service_name
  domains          = var.domains
}

module "cloud_run" {
  source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref=1467eaf0115f71613727212b0b51b3f99e699842"

  project_id = var.project_id

  name                  = var.name
  image                 = var.image
  ingress               = var.enable_gclb ? "internal-and-cloud-load-balancing" : "all"
  min_instances         = 1
  secrets               = [local.key_name]
  service_account_email = google_service_account.run_service_account.email
  args                  = ["webhook", "server"]
  service_iam = {
    admins     = var.service_iam.admins
    developers = toset(concat(var.service_iam.developers, [var.ci_service_account_member]))
    invokers   = toset(var.service_iam.invokers)
  }

  additional_service_annotations = {
    # GitHub webhooks call without authorization so the service
    # must allow unauthenticated requests to come through
    "run.googleapis.com/invoker-iam-disabled" : true
  }

  envvars = merge(
    var.envvars,
    {
      "KMS_APP_PRIVATE_KEY_ID" : "${google_kms_crypto_key.private_key.id}/cryptoKeyVersions/${var.kms_private_key_version_number}"
      "WEBHOOK_KEY_MOUNT_PATH" : local.key_mount_path
      "WEBHOOK_KEY_NAME" : local.key_name
    }
  )

  secret_envvars = {}

  secret_volumes = {
    "${local.key_mount_path}" : {
      name : "${local.key_name}",
      version : "latest",
    }
  }
}

# allow the ci service account to act as the cloud run service account
# this allows the ci service account to deploy new revisions for the
# cloud run service
resource "google_service_account_iam_member" "run_sa_ci_binding" {
  service_account_id = google_service_account.run_service_account.name
  role               = "roles/iam.serviceAccountUser"
  member             = var.ci_service_account_member
}
