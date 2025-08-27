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

func generateValidConfig() *Config {
	return &Config{
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
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutator func(c *Config)
		expErr  string
	}{
		{
			name:    "valid",
			mutator: nil,
		},
		{
			name:    "invalid_runner_label",
			mutator: func(c *Config) { c.RunnerLabel = "" },
			expErr:  "RUNNER_LABEL is required",
		},
		{
			name:    "invalid_environment",
			mutator: func(c *Config) { c.Environment = "invalid" },
			expErr:  `ENVIRONMENT must be one of 'production' or 'autopush', got "invalid"`,
		},
		{
			name:    "missing_github_app_id",
			mutator: func(c *Config) { c.GitHubAppID = "" },
			expErr:  "GITHUB_APP_ID is required",
		},
		{
			name:    "missing_webhook_key_mount_path",
			mutator: func(c *Config) { c.GitHubWebhookKeyMountPath = "" },
			expErr:  "WEBHOOK_KEY_MOUNT_PATH is required",
		},
		{
			name:    "missing_webhook_key_name",
			mutator: func(c *Config) { c.GitHubWebhookKeyName = "" },
			expErr:  "WEBHOOK_KEY_NAME is required",
		},
		{
			name:    "missing_kms_app_private_key_id",
			mutator: func(c *Config) { c.KMSAppPrivateKeyID = "" },
			expErr:  "KMS_APP_PRIVATE_KEY_ID is required",
		},
		{
			name:    "missing_runner_location",
			mutator: func(c *Config) { c.RunnerLocation = "" },
			expErr:  "RUNNER_LOCATION is required",
		},
		{
			name:    "missing_runner_project_id",
			mutator: func(c *Config) { c.RunnerProjectID = "" },
			expErr:  "RUNNER_PROJECT_ID is required",
		},
		{
			name:    "missing_runner_repository_id",
			mutator: func(c *Config) { c.RunnerRepositoryID = "" },
			expErr:  "RUNNER_REPOSITORY_ID is required",
		},
		{
			name:    "missing_runner_service_account",
			mutator: func(c *Config) { c.RunnerServiceAccount = "" },
			expErr:  "RUNNER_SERVICE_ACCOUNT is required",
		},
		{
			name:    "invalid_extra_runner_count",
			mutator: func(c *Config) { c.ExtraRunnerCount = "abc" },
			expErr:  "EXTRA_RUNNER_COUNT must be an integer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := generateValidConfig()
			if tc.mutator != nil {
				tc.mutator(cfg)
			}

			err := cfg.Validate()
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
