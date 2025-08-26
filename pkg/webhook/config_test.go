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
		name   string
		cfg    *Config
		expErr string
	}{
		{
			name: "valid",
			cfg:  generateValidConfig(),
		},
		{
			name: "invalid_runner_label",
			cfg: func() *Config {
				c := generateValidConfig()
				c.RunnerLabel = ""
				return c
			}(),
			expErr: "RUNNER_LABEL is required",
		},
		{
			name: "invalid_environment",
			cfg: func() *Config {
				c := generateValidConfig()
				c.Environment = "invalid"
				return c
			}(),
			expErr: `ENVIRONMENT must be one of 'production' or 'autopush', got "invalid"`,
		},
		{
			name: "missing_github_app_id",
			cfg: func() *Config {
				c := generateValidConfig()
				c.GitHubAppID = ""
				return c
			}(),
			expErr: "GITHUB_APP_ID is required",
		},
		{
			name: "missing_webhook_key_mount_path",
			cfg: func() *Config {
				c := generateValidConfig()
				c.GitHubWebhookKeyMountPath = ""
				return c
			}(),
			expErr: "WEBHOOK_KEY_MOUNT_PATH is required",
		},
		{
			name: "missing_webhook_key_name",
			cfg: func() *Config {
				c := generateValidConfig()
				c.GitHubWebhookKeyName = ""
				return c
			}(),
			expErr: "WEBHOOK_KEY_NAME is required",
		},
		{
			name: "missing_kms_app_private_key_id",
			cfg: func() *Config {
				c := generateValidConfig()
				c.KMSAppPrivateKeyID = ""
				return c
			}(),
			expErr: "KMS_APP_PRIVATE_KEY_ID is required",
		},
		{
			name: "missing_runner_location",
			cfg: func() *Config {
				c := generateValidConfig()
				c.RunnerLocation = ""
				return c
			}(),
			expErr: "RUNNER_LOCATION is required",
		},
		{
			name: "missing_runner_project_id",
			cfg: func() *Config {
				c := generateValidConfig()
				c.RunnerProjectID = ""
				return c
			}(),
			expErr: "RUNNER_PROJECT_ID is required",
		},
		{
			name: "missing_runner_repository_id",
			cfg: func() *Config {
				c := generateValidConfig()
				c.RunnerRepositoryID = ""
				return c
			}(),
			expErr: "RUNNER_REPOSITORY_ID is required",
		},
		{
			name: "missing_runner_service_account",
			cfg: func() *Config {
				c := generateValidConfig()
				c.RunnerServiceAccount = ""
				return c
			}(),
			expErr: "RUNNER_SERVICE_ACCOUNT is required",
		},
		{
			name: "invalid_extra_runner_count",
			cfg: func() *Config {
				c := generateValidConfig()
				c.ExtraRunnerCount = "abc"
				return c
			}(),
			expErr: "EXTRA_RUNNER_COUNT must be an integer",
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
