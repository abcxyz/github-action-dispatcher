locals {
  metric_root            = "run.googleapis.com"
  resource_type          = "cloud_run_revision"
  log_source_suffix      = "stdout"
  cloud_run_service_name = module.cloud_run.service_name

  duration_metric_details = {
    "job-queued-duration" : {
      description               = "Duration a GitHub Actions workflow job spent in the queue (from created to started)."
      severity_filter           = "INFO"
      target_action_event_name  = "in_progress" # The log is emitted when action is "in_progress"
      duration_field            = "jsonPayload.duration_queued_seconds"
      includes_conclusion_label = false
    }
    "job-in-progress-duration" : {
      description               = "Duration a GitHub Actions workflow job spent actively running (from started to completed)."
      severity_filter           = "INFO"
      target_action_event_name  = "completed" # The log is emitted when action is "completed"
      duration_field            = "jsonPayload.duration_in_progress_seconds"
      includes_conclusion_label = true
    },
    "job-total-duration" : {
      description               = "Total duration of a GitHub Actions workflow job (from creation to completion)."
      severity_filter           = "INFO"
      target_action_event_name  = "completed" # The log is emitted when action is "completed"
      duration_field            = "jsonPayload.duration_total_seconds"
      includes_conclusion_label = true
    }
  }

  counter_metric_details = {
    "job-queued-count" : {
      description               = "Counter of every GitHub Actions workflow job that is queued."
      severity_filter           = "INFO"
      target_action_event_name  = "queued"
      includes_conclusion_label = false
    }
    "job-in-progress-count" : {
      description               = "Counter of every GitHub Actions workflow job that is in progress."
      severity_filter           = "INFO"
      target_action_event_name  = "in_progress"
      includes_conclusion_label = false
    },
    "job-completed-count" : {
      description               = "Counter of every GitHub Actions workflow job that is completed."
      severity_filter           = "INFO"
      target_action_event_name  = "completed"
      includes_conclusion_label = true
    },
  }
}

resource "google_logging_metric" "github_job_duration" {
  for_each = local.duration_metric_details

  project = var.project_id

  name            = "${replace(local.cloud_run_service_name, "-", "_")}-${replace(each.key, "-", "_")}" # e.g., "service_name-metric_key"
  description     = each.value.description
  value_extractor = "EXTRACT(${each.value.duration_field})"

  filter = <<-EOT
    resource.type="${local.resource_type}"
    resource.labels.service_name="${local.cloud_run_service_name}"
    logName="projects/${var.project_id}/logs/${local.metric_root}%2F${local.log_source_suffix}"
    severity="${each.value.severity_filter}"
    jsonPayload.action_event_name="${each.value.target_action_event_name}"
    ${each.value.duration_field}:*
    ${each.value.includes_conclusion_label ? "jsonPayload.conclusion:*" : ""}
  EOT

  bucket_options {
    exponential_buckets {
      growth_factor      = 1.11 # Each subsequent bucket will be 11% wider than the last.
      num_finite_buckets = 100  # Creates 100 finite buckets to cover the range.
      scale              = 1.0  # The lower bound of the first finite bucket is 1 second.
    }
  }

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "s"

    labels {
      key         = "location"
      value_type  = "STRING"
      description = "Location of the Cloud Run service instance."
    }
    labels {
      key         = "service_name"
      value_type  = "STRING"
      description = "Name of the Cloud Run service."
    }
    dynamic "labels" {
      for_each = each.value.includes_conclusion_label ? [1] : []
      content {
        key         = "conclusion"
        value_type  = "STRING"
        description = "The conclusion of the GitHub Actions workflow job (e.g., success, failure). Only present for 'completed' jobs."
      }
    }
  }

  label_extractors = merge(
    {
      "location"     = "EXTRACT(resource.labels.location)"
      "service_name" = "EXTRACT(resource.labels.service_name)"
    },
    each.value.includes_conclusion_label ? { "conclusion" = "EXTRACT(jsonPayload.conclusion)" } : {}
  )
}

resource "google_logging_metric" "github_job_counter" {
  for_each = local.counter_metric_details

  project = var.project_id

  name        = "${replace(local.cloud_run_service_name, "-", "_")}-${replace(each.key, "-", "_")}" # e.g., "service_name-metric_key"
  description = each.value.description

  filter = <<-EOT
    resource.type="${local.resource_type}"
    resource.labels.service_name="${local.cloud_run_service_name}"
    logName="projects/${var.project_id}/logs/${local.metric_root}%2F${local.log_source_suffix}"
    severity="${each.value.severity_filter}"
    jsonPayload.action_event_name="${each.value.target_action_event_name}"
    ${each.value.includes_conclusion_label ? "jsonPayload.conclusion:*" : ""}
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"

    labels {
      key         = "location"
      value_type  = "STRING"
      description = "Location of the Cloud Run service instance."
    }
    labels {
      key         = "service_name"
      value_type  = "STRING"
      description = "Name of the Cloud Run service."
    }
    dynamic "labels" {
      for_each = each.value.includes_conclusion_label ? [1] : []
      content {
        key         = "conclusion"
        value_type  = "STRING"
        description = "The conclusion of the GitHub Actions workflow job (e.g., success, failure). Only present for 'completed' jobs."
      }
    }
  }

  label_extractors = merge(
    {
      "location"     = "EXTRACT(resource.labels.location)"
      "service_name" = "EXTRACT(resource.labels.service_name)"
    },
    each.value.includes_conclusion_label ? { "conclusion" = "EXTRACT(jsonPayload.conclusion)" } : {}
  )
}

resource "google_logging_metric" "error_processing_event_count" {
  project = var.project_id

  name        = "${replace(local.cloud_run_service_name, "-", "_")}-error_processing_event_count"
  description = "Capture instances of error processing events message in action dispatcher logs"

  filter = <<-EOT
    resource.type = "${local.resource_type}"
    resource.labels.service_name="${local.cloud_run_service_name}"
    logName="projects/${var.project_id}/logs/${local.metric_root}%2F${local.log_source_suffix}"
    resource.labels.location = "${var.region}"
    severity=ERROR
    jsonPayload.message="error processing request"
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"

    labels {
      key         = "location"
      value_type  = "STRING"
      description = "Location of the Cloud Run service instance."
    }
    labels {
      key         = "service_name"
      value_type  = "STRING"
      description = "Name of the Cloud Run service."
    }
  }

  label_extractors = {
    "location"     = "EXTRACT(resource.labels.location)"
    "service_name" = "EXTRACT(resource.labels.service_name)"
  }
}
