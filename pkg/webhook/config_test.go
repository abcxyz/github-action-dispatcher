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
		RunnerTimeoutSeconds:      "300",
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
		{
			name:    "invalid_runner_timeout_seconds_not_int",
			mutator: func(c *Config) { c.RunnerTimeoutSeconds = "abc" },
			expErr:  "RUNNER_TIMEOUT_SECONDS must be an integer",
		},
		{
			name:    "invalid_runner_timeout_seconds_too_low",
			mutator: func(c *Config) { c.RunnerTimeoutSeconds = "299" },
			expErr:  "RUNNER_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 299",
		},
		{
			name:    "invalid_runner_timeout_seconds_too_high",
			mutator: func(c *Config) { c.RunnerTimeoutSeconds = "86401" },
			expErr:  "RUNNER_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:    "invalid_runner_timeout_seconds_negative",
			mutator: func(c *Config) { c.RunnerTimeoutSeconds = "-1" },
			expErr:  "RUNNER_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got -1",
		},
		{
			name:    "valid_runner_timeout_seconds_min",
			mutator: func(c *Config) { c.RunnerTimeoutSeconds = "300" },
		},
		{
			name:    "valid_runner_timeout_seconds_max",
			mutator: func(c *Config) { c.RunnerTimeoutSeconds = "86400" },
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

func TestValidateRunnerTimeout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		value  string
		exp    int
		expErr string
	}{
		{
			name:   "valid_min",
			value:  "300",
			exp:    300,
			expErr: "",
		},
		{
			name:   "valid_max",
			value:  "86400",
			exp:    86400,
			expErr: "",
		},
		{
			name:   "valid_mid_range",
			value:  "1800", // 30 minutes
			exp:    1800,
			expErr: "",
		},
		{
			name:   "invalid_not_an_integer",
			value:  "abc",
			exp:    0,
			expErr: "RUNNER_TIMEOUT_SECONDS must be an integer",
		},
		{
			name:   "invalid_too_low",
			value:  "299",
			exp:    0,
			expErr: "RUNNER_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 299",
		},
		{
			name:   "invalid_too_high",
			value:  "86401",
			exp:    0,
			expErr: "RUNNER_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:   "invalid_negative_number",
			value:  "-1",
			exp:    0,
			expErr: "RUNNER_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got -1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateRunnerTimeout(tc.value)
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Errorf("validateRunnerTimeout(%q) got unexpected error diff: %v", tc.value, diff)
			}
			if got != tc.exp {
				t.Errorf("validateRunnerTimeout(%q) got %d, want %d", tc.value, got, tc.exp)
			}
		})
	}
}
