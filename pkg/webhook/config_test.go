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
		Environment:                   "production",
		ExtraRunnerCount:              0,
		GitHubAppID:                   "test-app-id",
		GitHubWebhookKeyMountPath:     "/tmp",
		GitHubWebhookKeyName:          "test-key",
		KMSAppPrivateKeyID:            "test-kms-key",
		RunnerExecutionTimeoutSeconds: 3600,
		RunnerIdleTimeoutSeconds:      300,
		RunnerLocation:                "test-location",
		RunnerProjectID:               "test-project",
		RunnerRepositoryID:            "test-repo",
		RunnerServiceAccount:          "test-sa",
		RunnerLabelAliases: map[string]string{
			"self-hosted": "sh-ubuntu-latest",
		},
		SupportedRunnerLabels: []string{
			"sh-ubuntu-latest",
			"ubuntu-24.04-n2d-standard-2",
			"ubuntu-20.04-2c-8g",
		},
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
			name:    "invalid_extra_runner_count_negative",
			mutator: func(c *Config) { c.ExtraRunnerCount = -1 },
			expErr:  "EXTRA_RUNNER_COUNT must be non-negative, got -1",
		},
		{
			name:    "invalid_runner_idle_timeout_seconds_too_low",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = 299 },
			expErr:  "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 299",
		},
		{
			name:    "invalid_runner_idle_timeout_seconds_too_high",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = 86401 },
			expErr:  "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:    "invalid_runner_idle_timeout_seconds_negative",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = -1 },
			expErr:  "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got -1",
		},
		{
			name:    "valid_runner_idle_timeout_seconds_min",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = 300 },
		},
		{
			name:    "valid_runner_idle_timeout_seconds_max",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = 86400 },
		},
		{
			name:    "invalid_runner_execution_timeout_seconds_too_low",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = 3599 },
			expErr:  "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got 3599",
		},
		{
			name:    "invalid_runner_execution_timeout_seconds_too_high",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = 86401 },
			expErr:  "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:    "invalid_runner_execution_timeout_seconds_negative",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = -1 },
			expErr:  "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got -1",
		},
		{
			name:    "valid_runner_execution_timeout_seconds_min",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = 3600 },
		},
		{
			name:    "valid_runner_execution_timeout_seconds_max",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = 86400 },
		},

		{
			name:    "missing_runner_label_aliases",
			mutator: func(c *Config) { c.RunnerLabelAliases = nil },
			expErr:  "RUNNER_LABEL_ALIASES must be provided",
		},
		{
			name:    "missing_supported_runner_labels",
			mutator: func(c *Config) { c.SupportedRunnerLabels = nil },
			expErr:  "SUPPORTED_RUNNER_LABELS must be provided",
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
