resource "random_id" "kms" {
  byte_length = 1
}

resource "google_kms_key_ring" "key_ring" {
  project = var.project_id

  name     = "${var.gad_prefix}-${random_id.kms.hex}-key-ring"
  location = var.region

  depends_on = [
    google_project_service.default["cloudkms.googleapis.com"],
  ]

  lifecycle {
    prevent_destroy = false
  }
}

resource "google_kms_crypto_key" "private_key" {
  name                          = "${var.gad_prefix}-${random_id.kms.hex}-pk"
  key_ring                      = google_kms_key_ring.key_ring.id
  purpose                       = "ASYMMETRIC_SIGN"
  skip_initial_version_creation = true

  version_template {
    algorithm = "RSA_SIGN_PKCS1_2048_SHA256"
  }

  lifecycle {
    prevent_destroy = false
  }
}

resource "google_kms_crypto_key_iam_member" "public_key_viewer" {
  for_each = {
    "gad_sa" = google_service_account.run_service_account.member
  }

  crypto_key_id = google_kms_crypto_key.private_key.id
  role          = "roles/cloudkms.publicKeyViewer"
  member        = each.value

  depends_on = [
    google_kms_crypto_key.private_key,
  ]
}

resource "google_kms_crypto_key_iam_member" "signer" {
  for_each = {
    "gad_sa" = google_service_account.run_service_account.member
  }

  crypto_key_id = google_kms_crypto_key.private_key.id
  role          = "roles/cloudkms.signer"
  member        = each.value

  depends_on = [
    google_kms_crypto_key.private_key,
  ]
}
