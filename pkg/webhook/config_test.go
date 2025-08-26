// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"testing"

	"github.com/abcxyz/pkg/testutil"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		cfg    *Config
		expErr string
	}{
		{
			name: "valid",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
				KMSAppPrivateKeyID:        "test-kms-key",
				RunnerLocation:            "test-location",
				RunnerProjectID:           "test-project",
				RunnerRepositoryID:        "test-repo",
				RunnerServiceAccount:      "test-sa",
				RunnerLabel:               "self-hosted",
				ExtraRunnerCount:          "0",
			},
		},
		{
			name: "invalid_environment",
			cfg: &Config{
				Environment: "invalid",
			},
			expErr: `ENVIRONMENT must be one of 'production' or 'autopush', got "invalid"`,
		},
		{
			name: "missing_github_app_id",
			cfg: &Config{
				Environment: "production",
			},
			expErr: "GITHUB_APP_ID is required",
		},
		{
			name: "missing_webhook_key_mount_path",
			cfg: &Config{
				Environment: "production",
				GitHubAppID: "test-app-id",
			},
			expErr: "WEBHOOK_KEY_MOUNT_PATH is required",
		},
		{
			name: "missing_webhook_key_name",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
			},
			expErr: "WEBHOOK_KEY_NAME is required",
		},
		{
			name: "missing_kms_app_private_key_id",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
			},
			expErr: "KMS_APP_PRIVATE_KEY_ID is required",
		},
		{
			name: "missing_runner_location",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
				KMSAppPrivateKeyID:        "test-kms-key",
			},
			expErr: "RUNNER_LOCATION is required",
		},
		{
			name: "missing_runner_project_id",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
				KMSAppPrivateKeyID:        "test-kms-key",
				RunnerLocation:            "test-location",
			},
			expErr: "RUNNER_PROJECT_ID is required",
		},
		{
			name: "missing_runner_repository_id",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
				KMSAppPrivateKeyID:        "test-kms-key",
				RunnerLocation:            "test-location",
				RunnerProjectID:           "test-project",
			},
			expErr: "RUNNER_REPOSITORY_ID is required",
		},
		{
			name: "missing_runner_service_account",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
				KMSAppPrivateKeyID:        "test-kms-key",
				RunnerLocation:            "test-location",
				RunnerProjectID:           "test-project",
				RunnerRepositoryID:        "test-repo",
			},
			expErr: "RUNNER_SERVICE_ACCOUNT is required",
		},
		{
			name: "invalid_extra_runner_count",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
				KMSAppPrivateKeyID:        "test-kms-key",
				RunnerLocation:            "test-location",
				RunnerProjectID:           "test-project",
				RunnerRepositoryID:        "test-repo",
				RunnerServiceAccount:      "test-sa",
				RunnerLabel:               "self-hosted",
				ExtraRunnerCount:          "abc",
			},
			expErr: "EXTRA_RUNNER_COUNT must be an integer",
		},
		{
			name: "invalid_runner_label",
			cfg: &Config{
				Environment:               "production",
				GitHubAppID:               "test-app-id",
				GitHubWebhookKeyMountPath: "/tmp",
				GitHubWebhookKeyName:      "test-key",
				KMSAppPrivateKeyID:        "test-kms-key",
				RunnerLocation:            "test-location",
				RunnerProjectID:           "test-project",
				RunnerRepositoryID:        "test-repo",
				RunnerServiceAccount:      "test-sa",
				RunnerLabel:               "",
				ExtraRunnerCount:          "0",
			},
			expErr: "RUNNER_LABEL is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
