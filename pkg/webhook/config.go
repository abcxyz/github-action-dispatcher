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
	"strconv"
	"strings"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/cfgloader"
	"github.com/abcxyz/pkg/cli"
)

const (
	minRunnerIdleTimeoutSeconds = 5 * 60       // 5 minutes
	maxRunnerIdleTimeoutSeconds = 24 * 60 * 60 // 24 hours

	minRunnerExecutionTimeoutSeconds = 1 * 60 * 60  // 1 hour
	maxRunnerExecutionTimeoutSeconds = 24 * 60 * 60 // 24 hours
)

// Config defines the set of environment variables required
// for running the webhook service.
type Config struct {
	Environment                   string `env:"ENVIRONMENT,default=production"`
	GitHubAPIBaseURL              string `env:"GITHUB_API_BASE_URL,default=https://api.github.com"`
	GitHubAppID                   string `env:"GITHUB_APP_ID,required"`
	GitHubWebhookKeyMountPath     string `env:"WEBHOOK_KEY_MOUNT_PATH,required"`
	GitHubWebhookKeyName          string `env:"WEBHOOK_KEY_NAME,required"`
	KMSAppPrivateKeyID            string `env:"KMS_APP_PRIVATE_KEY_ID,required"`
	Port                          string `env:"PORT,default=8080"`
	RunnerExecutionTimeoutSeconds string `env:"RUNNER_EXECUTION_TIMEOUT_SECONDS,default=3600"`
	RunnerIdleTimeoutSeconds      string `env:"RUNNER_IDLE_TIMEOUT_SECONDS,default=300"`
	RunnerImageName               string `env:"RUNNER_IMAGE_NAME,default=default-runner"`
	RunnerImageTag                string `env:"RUNNER_IMAGE_TAG,default=latest"`
	RunnerLocation                string `env:"RUNNER_LOCATION,required"`
	RunnerProjectID               string `env:"RUNNER_PROJECT_ID,required"`
	RunnerRepositoryID            string `env:"RUNNER_REPOSITORY_ID,required"`
	RunnerServiceAccount          string `env:"RUNNER_SERVICE_ACCOUNT,required"`
	ExtraRunnerCount              string `env:"EXTRA_RUNNER_COUNT,default=0"`
	RunnerWorkerPoolID            string `env:"RUNNER_WORKER_POOL_ID"`
	E2ETestRunID                  string `env:"E2ETestRunID"`
	RunnerLabel                   string `env:"RUNNER_LABEL,default=self-hosted"`
	EnableSelfHostedLabel         bool   `env:"ENABLE_SELF_HOSTED_LABEL,default=false"`
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

	if _, err := validateRunnerIdleTimeout(cfg.RunnerIdleTimeoutSeconds); err != nil {
		return err
	}

	if _, err := validateRunnerExecutionTimeout(cfg.RunnerExecutionTimeoutSeconds); err != nil {
		return err
	}

	if _, err := validateExtraRunnerCount(cfg.ExtraRunnerCount); err != nil {
		return err
	}

	if strings.TrimSpace(cfg.RunnerLabel) == "" {
		return fmt.Errorf("RUNNER_LABEL is required")
	}

	return nil
}

// NewConfig creates a new Config from environment variables.
func NewConfig(ctx context.Context) (*Config, error) {
	return newConfig(ctx, envconfig.OsLookuper())
}

func validateRunnerIdleTimeout(value string) (int, error) {
	num, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("RUNNER_IDLE_TIMEOUT_SECONDS must be an integer: %w", err)
	}

	if num < minRunnerIdleTimeoutSeconds || num > maxRunnerIdleTimeoutSeconds {
		return 0, fmt.Errorf("RUNNER_IDLE_TIMEOUT_SECONDS must be between %d (5 minutes) and %d (24 hours) seconds, got %d", minRunnerIdleTimeoutSeconds, maxRunnerIdleTimeoutSeconds, num)
	}

	return num, nil
}

func validateRunnerExecutionTimeout(value string) (int, error) {
	num, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("RUNNER_EXECUTION_TIMEOUT_SECONDS must be an integer: %w", err)
	}

	if num < minRunnerExecutionTimeoutSeconds || num > maxRunnerExecutionTimeoutSeconds {
		return 0, fmt.Errorf("RUNNER_EXECUTION_TIMEOUT_SECONDS must be between %d (1 hour) and %d (24 hours) seconds, got %d", minRunnerExecutionTimeoutSeconds, maxRunnerExecutionTimeoutSeconds, num)
	}

	return num, nil
}

func validateExtraRunnerCount(value string) (int, error) {
	if num, err := strconv.Atoi(value); err != nil {
		return 0, fmt.Errorf("EXTRA_RUNNER_COUNT must be an integer: %w", err)
	} else if num < 0 || num >= 10 {
		return 0, fmt.Errorf("EXTRA_RUNNER_COUNT must be in a range of [0,10)")
	} else {
		return num, nil
	}
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
		Name:   "e2e-test-run-id",
		Target: &cfg.E2ETestRunID,
		EnvVar: "E2E_TEST_RUN_ID",
		Usage:  `The unique ID for an E2E test run, used for tagging builds.`,
	})

	f.StringVar(&cli.StringVar{
		Name:    "environment",
		Target:  &cfg.Environment,
		EnvVar:  "ENVIRONMENT",
		Default: "production",
		Usage:   `The execution environment (e.g., "autopush", "production"). Controls environment-specific features.`,
	})

	f.StringVar(&cli.StringVar{
		Name:    "extra-runner-count",
		Target:  &cfg.ExtraRunnerCount,
		EnvVar:  "EXTRA_RUNNER_COUNT",
		Default: "0",
		Usage:   `How many extra runners to spawn per webhook. Used to create excess runners to avoid runner deficit. Must be in range [0,10).`,
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
		Name:    "runner-label",
		Target:  &cfg.RunnerLabel,
		EnvVar:  "RUNNER_LABEL",
		Default: "self-hosted",
		Usage:   `The single, exact label that the webhook will process for self-hosted runners.`,
	})

	f.StringVar(&cli.StringVar{
		Name:    "runner-idle-timeout-seconds",
		Target:  &cfg.RunnerIdleTimeoutSeconds,
		EnvVar:  "RUNNER_IDLE_TIMEOUT_SECONDS",
		Default: "300",
		Usage:   `The timeout for the runner in seconds. Must be between 300 (5 minutes) and 86400 (24 hours).`,
	})

	f.StringVar(&cli.StringVar{
		Name:    "runner-execution-timeout-seconds",
		Target:  &cfg.RunnerExecutionTimeoutSeconds,
		EnvVar:  "RUNNER_EXECUTION_TIMEOUT_SECONDS",
		Default: "3600",
		Usage:   `The timeout for the entire build in seconds. Must be between 3600 (1 hour) and 86400 (24 hours).`,
	})

	f.BoolVar(&cli.BoolVar{
		Name:    "enable-self-hosted-label",
		Target:  &cfg.EnableSelfHostedLabel,
		Usage:   "Enable to also allow self-hosted in addition to runner-label. Temporary until org registration is enabled.",
		Default: false,
		EnvVar:  "ENABLE_SELF_HOSTED_LABEL",
	})

	return set
}
