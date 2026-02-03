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

// Package webhook is the base webhook server for a github app's events specific to queued workflow jobs.
package webhook

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"
	"google.golang.org/api/option"

	"github.com/abcxyz/github-action-dispatcher/pkg/cloudbuild"
	gh "github.com/abcxyz/github-action-dispatcher/pkg/github"
	"github.com/abcxyz/github-action-dispatcher/pkg/version"
	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/healthcheck"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
)

// Server provides the server implementation.
type Server struct {
	cbc                           cloudbuild.Client
	ghc                           gh.Client
	e2eTestRunID                  string
	enableSelfHostedLabel         bool
	environment                   string
	extraRunnerCount              int
	ghAPIBaseURL                  string
	h                             *renderer.Renderer
	kmc                           KeyManagementClient
	runnerExecutionTimeoutSeconds int
	runnerIdleTimeoutSeconds      int
	runnerLabel                   string
	runnerLocation                string
	runnerProjectID               string
	runnerImageName               string
	runnerImageTag                string
	runnerRepositoryID            string
	runnerServiceAccount          string
	runnerWorkerPoolID            string
	webhookSecret                 []byte
	backoffInitialDelay           time.Duration
	maxRetryAttempts              int
}

// FileReader can read a file and return the content.
type FileReader interface {
	ReadFile(filename string) ([]byte, error)
}

// KeyManagementClient adheres to the interaction the webhook service has with a subset of Key Management APIs.
type KeyManagementClient interface {
	Close() error
	CreateSigner(ctx context.Context, kmsAppPrivateKeyID string) (*gcpkms.Signer, error)
}

// WebhookClientOptions encapsulate client config options as well as dependency implementation overrides.
type WebhookClientOptions struct {
	CloudBuildClientOpts    []option.ClientOption
	KeyManagementClientOpts []option.ClientOption

	OSFileReaderOverride        FileReader
	CloudBuildClientOverride    cloudbuild.Client
	GitHubClientOverride        gh.Client
	KeyManagementClientOverride KeyManagementClient
}

// NewServer creates a new HTTP server implementation that will handle
// receiving webhook payloads.
func NewServer(ctx context.Context, h *renderer.Renderer, cfg *Config, wco *WebhookClientOptions) (*Server, error) {
	fr := wco.OSFileReaderOverride
	if fr == nil {
		fr = NewOSFileReader()
	}

	webhookSecret, err := fr.ReadFile(fmt.Sprintf("%s/%s", cfg.GitHubWebhookKeyMountPath, cfg.GitHubWebhookKeyName))
	if err != nil {
		return nil, fmt.Errorf("failed to read webhook secret: %w", err)
	}

	kmc := wco.KeyManagementClientOverride
	if kmc == nil {
		km, err := NewKeyManagement(ctx, wco.KeyManagementClientOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create kms client: %w", err)
		}
		kmc = km
	}

	signer, err := kmc.CreateSigner(ctx, cfg.KMSAppPrivateKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to create app signer: %w", err)
	}

	ghc := wco.GitHubClientOverride
	if ghc == nil {
		options := []githubauth.Option{
			githubauth.WithBaseURL(cfg.GitHubAPIBaseURL),
		}
		appClient, err := githubauth.NewApp(cfg.GitHubAppID, signer, options...)
		if err != nil {
			return nil, fmt.Errorf("failed to create github app client: %w", err)
		}
		ghc = gh.NewClient(appClient, cfg.GitHubAPIBaseURL, cfg.BackoffInitialDelay, cfg.MaxRetryAttempts)
	}

	cbc := wco.CloudBuildClientOverride
	if cbc == nil {
		cb, err := cloudbuild.NewClient(ctx, cfg.BackoffInitialDelay, cfg.MaxRetryAttempts, wco.CloudBuildClientOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create cloudbuild client: %w", err)
		}
		cbc = cb
	}

	// cfg.Validate() is called before NewServer, safe to convert
	extraRunnerCount, _ := strconv.Atoi(cfg.ExtraRunnerCount)
	runnerIdleTimeoutSeconds, _ := strconv.Atoi(cfg.RunnerIdleTimeoutSeconds)
	runnerExecutionTimeoutSeconds, _ := strconv.Atoi(cfg.RunnerExecutionTimeoutSeconds)

	return &Server{
		backoffInitialDelay:           cfg.BackoffInitialDelay,
		cbc:                           cbc,
		e2eTestRunID:                  cfg.E2ETestRunID,
		enableSelfHostedLabel:         cfg.EnableSelfHostedLabel,
		environment:                   cfg.Environment,
		extraRunnerCount:              extraRunnerCount,
		ghAPIBaseURL:                  cfg.GitHubAPIBaseURL,
		ghc:                           ghc,
		h:                             h,
		kmc:                           kmc,
		maxRetryAttempts:              cfg.MaxRetryAttempts,
		runnerExecutionTimeoutSeconds: runnerExecutionTimeoutSeconds,
		runnerIdleTimeoutSeconds:      runnerIdleTimeoutSeconds,
		runnerLabel:                   cfg.RunnerLabel,
		runnerLocation:                cfg.RunnerLocation,
		runnerProjectID:               cfg.RunnerProjectID,
		runnerImageName:               cfg.RunnerImageName,
		runnerImageTag:                cfg.RunnerImageTag,
		runnerRepositoryID:            cfg.RunnerRepositoryID,
		runnerServiceAccount:          cfg.RunnerServiceAccount,
		runnerWorkerPoolID:            cfg.RunnerWorkerPoolID,
		webhookSecret:                 webhookSecret,
	}, nil
}

// Routes creates a ServeMux for all the routes that
// this Router supports.
func (s *Server) Routes(ctx context.Context) http.Handler {
	logger := logging.FromContext(ctx)
	mux := http.NewServeMux()
	mux.Handle("/healthz", healthcheck.HandleHTTPHealthCheck())
	mux.Handle("/webhook", s.handleWebhook())
	mux.Handle("/version", s.handleVersion())

	// Middleware
	root := logging.HTTPInterceptor(logger, s.runnerProjectID)(mux)

	return root
}

// handleVersion is a simple http.HandlerFunc that responds with version
// information for the server.
func (s *Server) handleVersion() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.h.RenderJSON(w, http.StatusOK, map[string]string{
			"version": version.HumanVersion,
		})
	})
}

// Close handles the graceful shutdown of the webhook server.
func (s *Server) Close() error {
	if err := s.kmc.Close(); err != nil {
		return fmt.Errorf("failed to shutdown kms client connection: %w", err)
	}

	if err := s.cbc.Close(); err != nil {
		return fmt.Errorf("failed to shutdown cloud build client connection: %w", err)
	}
	return nil
}
