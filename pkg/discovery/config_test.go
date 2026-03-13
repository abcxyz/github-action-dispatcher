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
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/pkg/testutil"
)

func generateValidConfig() *Config {
	return &Config{
		AllowedGithubOrgScopes:    "default",
		AllowedJobRunsOn:          "ubuntu-latest",
		AllowedPoolLocations:      "us-central1",
		AllowedPoolAvailabilities: "available,unavailable",
		AllowedPoolTypes:          "trusted,private",
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
				c.AllowedGithubOrgScopes = ""
			},
			expErr: "GCP_ALLOWED_PROJECT_LABEL_GH_ORG_SCOPE_VALUES must be provided",
		},
		{
			name: "missing_job_runs_on",
			mutator: func(c *Config) {
				c.AllowedJobRunsOn = ""
			},
			expErr: "GCP_ALLOWED_PROJECT_LABEL_JOB_RUNS_ON_VALUES must be provided",
		},
		{
			name: "missing_pool_location",
			mutator: func(c *Config) {
				c.AllowedPoolLocations = ""
			},
			expErr: "GCP_ALLOWED_PROJECT_LABEL_POOL_LOCATION_VALUES must be provided",
		},
		{
			name: "missing_pool_availability",
			mutator: func(c *Config) {
				c.AllowedPoolAvailabilities = ""
			},
			expErr: "GCP_ALLOWED_PROJECT_LABEL_POOL_AVAILABILITY_VALUES must be provided",
		},
		{
			name: "missing_pool_type",
			mutator: func(c *Config) {
				c.AllowedPoolTypes = ""
			},
			expErr: "GCP_ALLOWED_PROJECT_LABEL_POOL_TYPE_VALUES must be provided",
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

func TestNewConfig_Parsing(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GCP_ALLOWED_PROJECT_LABEL_GH_ORG_SCOPE_VALUES", "default,my-org")
	t.Setenv("GCP_ALLOWED_PROJECT_LABEL_JOB_RUNS_ON_VALUES", "ubuntu-latest,windows-latest")
	t.Setenv("GCP_ALLOWED_PROJECT_LABEL_POOL_LOCATION_VALUES", "us-central1,us-west1")
	t.Setenv("GCP_ALLOWED_PROJECT_LABEL_POOL_AVAILABILITY_VALUES", "available,unavailable")
	t.Setenv("GCP_ALLOWED_PROJECT_LABEL_POOL_TYPE_VALUES", "trusted,private")
	t.Setenv("GCP_FOLDER_ID", "12345")

	cfg, err := NewConfig(ctx)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	expAllowedGithubOrgScopes := []string{"default", "my-org"}
	if diff := cmp.Diff(expAllowedGithubOrgScopes, cfg.GetAllowedGithubOrgScopes()); diff != "" {
		t.Errorf("AllowedGithubOrgScopes (-want,+got):\n%s", diff)
	}
}
