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
	"fmt"
	"time"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/cfgloader"
)

const (
	jobRunsOnGCPProjectLabelKey        = "job-runs-on"
	poolLocationGCPProjectLabelKey     = "pool-location"
	githubOrgScopeGCPProjectLabelKey   = "gh-org-scope"
	poolAvailabilityGCPProjectLabelKey = "pool-availability"
	poolAvailabilityAvailable          = "AVAILABLE"
	poolAvailabilityUnavailable        = "UNAVAILABLE"
)

// Config defines the set of environment variables required
// for running the runner-discovery job.
type Config struct {
	GCPFolderID                    string        `env:"GCP_FOLDER_ID"`
	AllowedGithubOrgScopes         []string      `env:"GCP_RUNNER_ALLOWED_PROJECT_LABELS_GH_ORG_SCOPE,delimiter=,"`
	AllowedJobRunsOn               []string      `env:"GCP_RUNNER_ALLOWED_PROJECT_LABELS_JOB_RUNS_ON,delimiter=,"`
	AllowedPoolLocations           []string      `env:"GCP_RUNNER_ALLOWED_PROJECT_LABELS_POOL_LOCATION,delimiter=,"`
	AllowedPoolAvailabilities      []string      `env:"GCP_RUNNER_ALLOWED_PROJECT_LABELS_POOL_AVAILABILITY,delimiter=,"`
	MaxRetryAttempts               int           `env:"MAX_RETRY_ATTEMPTS,default=3"`
	BackoffInitialDelay            time.Duration `env:"BACKOFF_INITIAL_DELAY,default=500ms"`
	RunnerRegistryDefaultKeyPrefix string        `env:"RUNNER_REGISTRY_DEFAULT_KEY_PREFIX,default=default"`
}

// Validate validates the runner-discovery config after load.
func (cfg *Config) Validate() error {
	if cfg.GCPFolderID == "" {
		return fmt.Errorf("GCP_FOLDER_ID must be provided")
	}
	if len(cfg.AllowedGithubOrgScopes) == 0 {
		return fmt.Errorf("GCP_RUNNER_ALLOWED_PROJECT_LABELS_GH_ORG_SCOPE must be provided")
	}
	if len(cfg.AllowedJobRunsOn) == 0 {
		return fmt.Errorf("GCP_RUNNER_ALLOWED_PROJECT_LABELS_JOB_RUNS_ON must be provided")
	}
	if len(cfg.AllowedPoolLocations) == 0 {
		return fmt.Errorf("GCP_RUNNER_ALLOWED_PROJECT_LABELS_POOL_LOCATION must be provided")
	}
	if len(cfg.AllowedPoolAvailabilities) == 0 {
		return fmt.Errorf("GCP_RUNNER_ALLOWED_PROJECT_LABELS_POOL_AVAILABILITY must be provided")
	}

	return nil
}

// NewConfig creates a new Config from environment variables.
func NewConfig(ctx context.Context) (*Config, error) {
	return newConfig(ctx, envconfig.OsLookuper())
}

func newConfig(ctx context.Context, lu envconfig.Lookuper) (*Config, error) {
	var cfg Config
	if err := cfgloader.Load(ctx, &cfg, cfgloader.WithLookuper(lu)); err != nil {
		return nil, fmt.Errorf("failed to parse runner-discovery config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}
	return &cfg, nil
}
