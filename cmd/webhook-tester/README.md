# Webhook Tester

This tool is a command-line application designed to test the GitHub Actions dispatcher webhook. It simulates a `workflow_job` event from GitHub, signs it with a webhook secret, and sends it to a specified webhook URL.

## Features

-   Sends a realistic `workflow_job` payload.
-   Automatically fetches the webhook secret from Google Secret Manager.
-   Calculates the required SHA256 signature for the payload.
-   Can be used for local testing or integrated into CI/CD for end-to-end tests.
-   Supports authentication via OIDC tokens for invoking protected Cloud Run services.
-   Includes an option to poll and verify that the corresponding Cloud Build job was triggered.

## Usage

### Prerequisites

-   Go 1.22 or later.
-   Authenticated to Google Cloud with permissions to access the specified secret in Secret Manager.

### Flags

| Flag                | Description                                                                          | Required |
| ------------------- | ------------------------------------------------------------------------------------ | -------- |
| `--webhook-url`     | The URL of the webhook to test.                                                      | Yes      |
| `--secret-name`     | The full resource name of the secret in Secret Manager.                              | Yes      |
| `--installation-id` | The ID of the GitHub App installation.                                               | Yes      |
| `--project-id`      | The GCP project ID for the integration environment.                                  | Yes      |
| `--run-id`          | The unique GitHub Actions workflow run ID.                                           | Yes      |
| `--id-token`        | The OIDC ID token for authenticating to the webhook service.                         | No       |
| `--signature`       | A custom signature to use for the webhook payload. If empty, it will be calculated.  | No       |
| `--verify-runner`   | If true, verify the runner is online instead of the build.                           | No       |
| `--github-owner`    | The GitHub owner (organization). Required with `--verify-runner`.                    | No       |
| `--github-repo`     | The GitHub repository name. Required with `--verify-runner`.                         | No       |
| `--github-token`    | The GitHub token for authenticating to the API.                                      | No       |

### Example

```bash
go run ./cmd/webhook-tester \
  --webhook-url "https://your-cloud-run-service.a.run.app/webhook" \
  --secret-name "projects/your-gcp-project/secrets/your-webhook-secret/versions/latest" \
  --installation-id 12345678 \
  --project-id "your-gcp-project" \
  --run-id 987654321
```

### Testing with an Invalid Signature

You can test the webhook's signature validation by providing a deliberately incorrect signature:

```bash
go run ./cmd/webhook-tester \
  --webhook-url "https://your-cloud-run-service.a.run.app/webhook" \
  --secret-name "projects/your-gcp-project/secrets/your-webhook-secret/versions/latest" \
  --installation-id 12345678 \
  --project-id "your-gcp-project" \
  --run-id 987654321 \
  --signature "invalid-signature"
```

This should result in an authorization error from the webhook service.
