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

package discovery

import (
	"testing"

	"github.com/abcxyz/pkg/testutil"
)

func generateValidConfig() *Config {
	return &Config{
		AllowedGithubOrgScopes:    []string{"default"},
		AllowedJobRunsOn:          []string{"ubuntu-latest"},
		AllowedPoolLocations:      []string{"us-central1"},
		AllowedPoolAvailabilities: []string{poolAvailabilityAvailable, poolAvailabilityUnavailable},
		GCPFolderID:               "1234567890",
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
			name: "missing_gcp_folder_id",
			mutator: func(c *Config) {
				c.GCPFolderID = ""
			},
			expErr: "GCP_FOLDER_ID must be provided",
		},
		{
			name: "missing_gh_org_scope",
			mutator: func(c *Config) {
				c.AllowedGithubOrgScopes = nil
			},
			expErr: "GCP_RUNNER_ALLOWED_PROJECT_LABELS_GH_ORG_SCOPE must be provided",
		},
		{
			name: "missing_job_runs_on",
			mutator: func(c *Config) {
				c.AllowedJobRunsOn = nil
			},
			expErr: "GCP_RUNNER_ALLOWED_PROJECT_LABELS_JOB_RUNS_ON must be provided",
		},
		{
			name: "missing_pool_location",
			mutator: func(c *Config) {
				c.AllowedPoolLocations = nil
			},
			expErr: "GCP_RUNNER_ALLOWED_PROJECT_LABELS_POOL_LOCATION must be provided",
		},
		{
			name: "missing_pool_availability",
			mutator: func(c *Config) {
				c.AllowedPoolAvailabilities = nil
			},
			expErr: "GCP_RUNNER_ALLOWED_PROJECT_LABELS_POOL_AVAILABILITY must be provided",
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
