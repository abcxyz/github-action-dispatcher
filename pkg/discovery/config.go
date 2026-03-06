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
	"strings"
	"time"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/cfgloader"
)

const (
	jobRunsOnGCPProjectLabelKey        = "job-runs-on"
	poolLocationGCPProjectLabelKey     = "pool-location"
	githubOrgScopeGCPProjectLabelKey   = "gh-org-scope"
	poolAvailabilityGCPProjectLabelKey = "pool-availability"
	poolAvailabilityAvailable          = "available"
	poolAvailabilityUnavailable        = "unavailable"
)

// Config defines the set of environment variables required
// for running the runner-discovery job.
type Config struct {
	GCPFolderID                    string        `env:"GCP_FOLDER_ID"`
	AllowedGithubOrgScopes         string        `env:"GCP_ALLOWED_PROJECT_LABEL_GH_ORG_SCOPE_VALUES"`
	AllowedJobRunsOn               string        `env:"GCP_ALLOWED_PROJECT_LABEL_JOB_RUNS_ON_VALUES"`
	AllowedPoolLocations           string        `env:"GCP_ALLOWED_PROJECT_LABEL_POOL_LOCATION_VALUES"`
	AllowedPoolAvailabilities      string        `env:"GCP_ALLOWED_PROJECT_LABEL_POOL_AVAILABILITY_VALUES"`
	MaxRetryAttempts               int           `env:"MAX_RETRY_ATTEMPTS,default=3"`
	BackoffInitialDelay            time.Duration `env:"BACKOFF_INITIAL_DELAY,default=500ms"`
	RunnerRegistryDefaultKeyPrefix string        `env:"RUNNER_REGISTRY_DEFAULT_KEY_PREFIX,default=default"`
}

// Validate validates the runner-discovery config after load.
func (cfg *Config) Validate() error {
	if cfg.GCPFolderID == "" {
		return fmt.Errorf("GCP_FOLDER_ID must be provided")
	}
	if cfg.AllowedGithubOrgScopes == "" {
		return fmt.Errorf("GCP_ALLOWED_PROJECT_LABEL_GH_ORG_SCOPE_VALUES must be provided")
	}
	if cfg.AllowedJobRunsOn == "" {
		return fmt.Errorf("GCP_ALLOWED_PROJECT_LABEL_JOB_RUNS_ON_VALUES must be provided")
	}
	if cfg.AllowedPoolLocations == "" {
		return fmt.Errorf("GCP_ALLOWED_PROJECT_LABEL_POOL_LOCATION_VALUES must be provided")
	}
	if cfg.AllowedPoolAvailabilities == "" {
		return fmt.Errorf("GCP_ALLOWED_PROJECT_LABEL_POOL_AVAILABILITY_VALUES must be provided")
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

func (c *Config) GetAllowedGithubOrgScopes() []string {
	return strings.Split(c.AllowedGithubOrgScopes, ",")
}

func (c *Config) GetAllowedJobRunsOn() []string {
	return strings.Split(c.AllowedJobRunsOn, ",")
}

func (c *Config) GetAllowedPoolLocations() []string {
	return strings.Split(c.AllowedPoolLocations, ",")
}

func (c *Config) GetAllowedPoolAvailabilities() []string {
	return strings.Split(c.AllowedPoolAvailabilities, ",")
}
