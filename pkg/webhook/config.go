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
	"context"
	"fmt"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/cfgloader"
	"github.com/abcxyz/pkg/cli"
)

// Config defines the set of environment variables required
// for running the webhook service.
type Config struct {
	Environment               string `env:"ENVIRONMENT,default=production"`
	GitHubAPIBaseURL          string `env:"GITHUB_API_BASE_URL,default=https://api.github.com"`
	GitHubAppID               string `env:"GITHUB_APP_ID,required"`
	GitHubWebhookKeyMountPath string `env:"WEBHOOK_KEY_MOUNT_PATH,required"`
	GitHubWebhookKeyName      string `env:"WEBHOOK_KEY_NAME,required"`
	KMSAppPrivateKeyID        string `env:"KMS_APP_PRIVATE_KEY_ID,required"`
	Port                      string `env:"PORT,default=8080"`
	RunnerImageName           string `env:"RUNNER_IMAGE_NAME,default=default-runner"`
	RunnerImageTag            string `env:"RUNNER_IMAGE_TAG,default=latest"`
	RunnerLocation            string `env:"RUNNER_LOCATION,required"`
	RunnerProjectID           string `env:"RUNNER_PROJECT_ID,required"`
	RunnerRepositoryID        string `env:"RUNNER_REPOSITORY_ID,required"`
	RunnerServiceAccount      string `env:"RUNNER_SERVICE_ACCOUNT,required"`
	RunnerWorkerPoolID        string `env:"RUNNER_WORKER_POOL_ID"`
	RunnerJWTSecret           string `env:"RUNNER_JWT_SECRET,required"`
}

// Validate validates the webhook config after load.
func (cfg *Config) Validate() error {
	if cfg.Environment != "production" && cfg.Environment != "autopush" {
		return fmt.Errorf("ENVIRONMENT must be one of 'production' or 'autopush', got %q", cfg.Environment)
	}

	if cfg.GitHubAppID == "" {
		return fmt.Errorf("GITHUB_APP_ID is required")
	}

	if cfg.GitHubWebhookKeyMountPath == "" {
		return fmt.Errorf("WEBHOOK_KEY_MOUNT_PATH is required")
	}

	if cfg.GitHubWebhookKeyName == "" {
		return fmt.Errorf("WEBHOOK_KEY_NAME is required")
	}

	if cfg.KMSAppPrivateKeyID == "" {
		return fmt.Errorf("KMS_APP_PRIVATE_KEY_ID is required")
	}

	if cfg.RunnerLocation == "" {
		return fmt.Errorf("RUNNER_LOCATION is required")
	}

	if cfg.RunnerProjectID == "" {
		return fmt.Errorf("RUNNER_PROJECT_ID is required")
	}

	if cfg.RunnerRepositoryID == "" {
		return fmt.Errorf("RUNNER_REPOSITORY_ID is required")
	}

	if cfg.RunnerServiceAccount == "" {
		return fmt.Errorf("RUNNER_SERVICE_ACCOUNT is required")
	}

	if cfg.RunnerJWTSecret == "" {
		return fmt.Errorf("RUNNER_JWT_SECRET is required")
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
		return nil, fmt.Errorf("failed to parse webhook config: %w", err)
	}
	return &cfg, nil
}

// ToFlags binds the config to the [cli.FlagSet] and returns it.
func (cfg *Config) ToFlags(set *cli.FlagSet) *cli.FlagSet {
	f := set.NewSection("COMMON SERVER OPTIONS")

	f.StringVar(&cli.StringVar{
		Name:    "environment",
		Target:  &cfg.Environment,
		EnvVar:  "ENVIRONMENT",
		Default: "production",
		Usage:   `The execution environment (e.g., "autopush", "production"). Controls environment-specific features.`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "runner-location",
		Target: &cfg.RunnerLocation,
		EnvVar: "RUNNER_LOCATION",
		Usage:  `The location used for the Cloud Build build.`,
	})

	f.StringVar(&cli.StringVar{
		Name:    "github-api-base-url",
		Target:  &cfg.GitHubAPIBaseURL,
		EnvVar:  "GITHUB_API_BASE_URL",
		Default: "https://api.github.com",
		Usage:   `The GitHub API URL.`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "github-app-id",
		Target: &cfg.GitHubAppID,
		EnvVar: "GITHUB_APP_ID",
		Usage:  `The provisioned GitHub App reference.`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "kms-app-private-key-id",
		Target: &cfg.KMSAppPrivateKeyID,
		EnvVar: "KMS_APP_PRIVATE_KEY_ID",
		Usage:  `The KMS private key path in the form "projects/<project_id>/locations/<location>/keyRings/<key_ring_name>/cryptoKeys/<key_name>/cryptoKeyVersions/<version>".`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "runner-project-id",
		Target: &cfg.RunnerProjectID,
		EnvVar: "RUNNER_PROJECT_ID",
		Usage:  `Google Cloud project ID where the runner will execute.`,
	})

	f.StringVar(&cli.StringVar{
		Name:    "port",
		Target:  &cfg.Port,
		EnvVar:  "PORT",
		Default: "8080",
		Usage:   `The port the retry server listens to.`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "github-webhook-key-mount-path",
		Target: &cfg.GitHubWebhookKeyMountPath,
		EnvVar: "WEBHOOK_KEY_MOUNT_PATH",
		Usage:  `GitHub webhook key mount path.`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "github-webhook-key-name",
		Target: &cfg.GitHubWebhookKeyName,
		EnvVar: "WEBHOOK_KEY_NAME",
		Usage:  `GitHub webhook key name.`,
	})

	f.StringVar(&cli.StringVar{
		Name:    "runner-image-name",
		Target:  &cfg.RunnerImageName,
		EnvVar:  "RUNNER_IMAGE_NAME",
		Default: "default-runner",
		Usage:   `The runner image name.`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "runner-image-tag",
		Target: &cfg.RunnerImageTag,
		EnvVar: "RUNNER_IMAGE_TAG",
		Usage:  `The runner image tag to pull`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "runner-repository-id",
		Target: &cfg.RunnerRepositoryID,
		EnvVar: "RUNNER_REPOSITORY_ID",
		Usage:  `The GAR repository that holds the runner image`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "runner-service-account",
		Target: &cfg.RunnerServiceAccount,
		EnvVar: "RUNNER_SERVICE_ACCOUNT",
		Usage:  `The service account the runner should execute as`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "runner-worker-pool-id",
		Target: &cfg.RunnerWorkerPoolID,
		EnvVar: "RUNNER_WORKER_POOL_ID",
		Usage:  `The private runner worker pool ID`,
	})

	f.StringVar(&cli.StringVar{
		Name:   "runner-jwt-secret",
		Target: &cfg.RunnerJWTSecret,
		EnvVar: "RUNNER_JWT_SECRET",
		Usage:  `The secret for signing and validating runner JWTs.`,
	})

	return set
}
