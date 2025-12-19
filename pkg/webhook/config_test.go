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
		ExtraRunnerCount:              "0",
		GitHubAppID:                   "test-app-id",
		GitHubAppInstallationID:       "123",
		GitHubWebhookKeyMountPath:     "/tmp",
		GitHubWebhookKeyName:          "test-key",
		KMSAppPrivateKeyID:            "test-kms-key",
		RunnerExecutionTimeoutSeconds: "3600",
		RunnerIdleTimeoutSeconds:      "300",
		RunnerLabel:                   "self-hosted",
		ExternalRunnerEndpoint:        "https://example.com/create",
		IAPServiceAudience:            "audience",
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
			name:    "missing_github_app_installation_id",
			mutator: func(c *Config) { c.GitHubAppInstallationID = "" },
			expErr:  "GITHUB_APP_INSTALLATION_ID is required",
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
			name:    "missing_external_runner_endpoint",
			mutator: func(c *Config) { c.ExternalRunnerEndpoint = "" },
			expErr:  "EXTERNAL_RUNNER_ENDPOINT is required",
		},
		{
			name:    "missing_iap_service_audience",
			mutator: func(c *Config) { c.IAPServiceAudience = "" },
			expErr:  "IAP_SERVICE_AUDIENCE is required",
		},
		{
			name:    "invalid_external_runner_endpoint_url",
			mutator: func(c *Config) { c.ExternalRunnerEndpoint = "invalid-url" },
			expErr:  "EXTERNAL_RUNNER_ENDPOINT must be a valid URL",
		},
		{
			name:    "invalid_extra_runner_count",
			mutator: func(c *Config) { c.ExtraRunnerCount = "abc" },
			expErr:  "EXTRA_RUNNER_COUNT must be an integer",
		},
		{
			name:    "invalid_runner_idle_timeout_seconds_not_int",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = "abc" },
			expErr:  "RUNNER_IDLE_TIMEOUT_SECONDS must be an integer",
		},
		{
			name:    "invalid_runner_idle_timeout_seconds_too_low",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = "299" },
			expErr:  "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 299",
		},
		{
			name:    "invalid_runner_idle_timeout_seconds_too_high",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = "86401" },
			expErr:  "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:    "invalid_runner_idle_timeout_seconds_negative",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = "-1" },
			expErr:  "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got -1",
		},
		{
			name:    "valid_runner_idle_timeout_seconds_min",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = "300" },
		},
		{
			name:    "valid_runner_idle_timeout_seconds_max",
			mutator: func(c *Config) { c.RunnerIdleTimeoutSeconds = "86400" },
		},
		{
			name:    "invalid_runner_execution_timeout_seconds_not_int",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = "abc" },
			expErr:  "RUNNER_EXECUTION_TIMEOUT_SECONDS must be an integer",
		},
		{
			name:    "invalid_runner_execution_timeout_seconds_too_low",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = "3599" },
			expErr:  "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got 3599",
		},
		{
			name:    "invalid_runner_execution_timeout_seconds_too_high",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = "86401" },
			expErr:  "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:    "valid_runner_execution_timeout_seconds_min",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = "3600" },
		},
		{
			name:    "valid_runner_execution_timeout_seconds_max",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = "86400" },
		},
		{
			name:    "invalid_runner_execution_timeout_seconds_negative",
			mutator: func(c *Config) { c.RunnerExecutionTimeoutSeconds = "-1" },
			expErr:  "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got -1",
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

func TestValidateRunnerIdleTimeout(t *testing.T) {
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
			expErr: "RUNNER_IDLE_TIMEOUT_SECONDS must be an integer",
		},
		{
			name:   "invalid_too_low",
			value:  "299",
			exp:    0,
			expErr: "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 299",
		},
		{
			name:   "invalid_too_high",
			value:  "86401",
			exp:    0,
			expErr: "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:   "invalid_negative_number",
			value:  "-1",
			exp:    0,
			expErr: "RUNNER_IDLE_TIMEOUT_SECONDS must be between 300 (5 minutes) and 86400 (24 hours) seconds, got -1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateRunnerIdleTimeout(tc.value)
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Errorf("validateRunnerIdleTimeout(%q) got unexpected error diff: %v", tc.value, diff)
			}
			if got != tc.exp {
				t.Errorf("validateRunnerIdleTimeout(%q) got %d, want %d", tc.value, got, tc.exp)
			}
		})
	}
}

func TestValidateRunnerExecutionTimeout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		value  string
		exp    int
		expErr string
	}{
		{
			name:   "valid_min",
			value:  "3600",
			exp:    3600,
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
			value:  "7200", // 2 hours
			exp:    7200,
			expErr: "",
		},
		{
			name:   "invalid_not_an_integer",
			value:  "abc",
			exp:    0,
			expErr: "RUNNER_EXECUTION_TIMEOUT_SECONDS must be an integer",
		},
		{
			name:   "invalid_too_low",
			value:  "3599",
			exp:    0,
			expErr: "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got 3599",
		},
		{
			name:   "invalid_too_high",
			value:  "86401",
			exp:    0,
			expErr: "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got 86401",
		},
		{
			name:   "invalid_negative_number",
			value:  "-1",
			exp:    0,
			expErr: "RUNNER_EXECUTION_TIMEOUT_SECONDS must be between 3600 (1 hour) and 86400 (24 hours) seconds, got -1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateRunnerExecutionTimeout(tc.value)
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Errorf("validateRunnerExecutionTimeout(%q) got unexpected error diff: %v", tc.value, diff)
			}
			if got != tc.exp {
				t.Errorf("validateRunnerExecutionTimeout(%q) got %d, want %d", tc.value, got, tc.exp)
			}
		})
	}
}
