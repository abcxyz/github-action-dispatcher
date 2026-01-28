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

	"github.com/abcxyz/github-action-dispatcher/pkg/retry"
	"github.com/abcxyz/pkg/cfgloader"
)

// Config defines the set of environment variables required
// for running the runner-discovery job.
type Config struct {
	GCPFolderID              string        `env:"GCP_FOLDER_ID"`
	LabelQuery               []string      `env:"LABEL_QUERY"`
	RetryHTTPMaxAttempts     int           `env:"RETRY_HTTP_MAX_ATTEMPTS,default=3"`
	RetryBackoffInitialDelay time.Duration `env:"RETRY_BACKOFF_INITIAL_DELAY"`
	RetryBackoffMaxDelay     time.Duration `env:"RETRY_BACKOFF_MAX_DELAY"`
	RetryBackoffMultiplier   float64       `env:"RETRY_BACKOFF_MULTIPLIER"`

	// Retry is the configuration for retryable operations. This field is populated
	// after envconfig loads the other fields.
	Retry *retry.BackoffConfig
}

// Validate validates the runner-discovery config after load.
func (cfg *Config) Validate() error {
	if cfg.GCPFolderID == "" {
		return fmt.Errorf("GCP_FOLDER_ID must be provided")
	}
	if len(cfg.LabelQuery) == 0 {
		return fmt.Errorf("LABEL_QUERY must be provided")
	}
	return nil
}

// NewConfig creates a new Config from environment variables.
func NewConfig(ctx context.Context) (*Config, error) {
	return newConfig(ctx, envconfig.OsLookuper())
}

func newConfig(ctx context.Context, lu envconfig.Lookuper) (*Config, error) {
	var cfg Config
	// Pre-fill the retry config with defaults, which can be overridden by env.
	defaultRetry := retry.DefaultBackoffConfig()
	cfg.RetryBackoffInitialDelay = defaultRetry.Initial
	cfg.RetryBackoffMaxDelay = defaultRetry.Max
	cfg.RetryBackoffMultiplier = defaultRetry.Multiplier

	if err := cfgloader.Load(ctx, &cfg, cfgloader.WithLookuper(lu)); err != nil {
		return nil, fmt.Errorf("failed to parse runner-discovery config: %w", err)
	}

	cfg.Retry = &retry.BackoffConfig{
		Initial:    cfg.RetryBackoffInitialDelay,
		Max:        cfg.RetryBackoffMaxDelay,
		Multiplier: cfg.RetryBackoffMultiplier,
	}
	return &cfg, nil
}
