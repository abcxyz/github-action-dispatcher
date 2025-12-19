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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/abcxyz/github-action-dispatcher/pkg/version"
	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/healthcheck"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"
	"google.golang.org/api/option"
)

// Server provides the server implementation.
type Server struct {
	appClient                     *githubauth.App
	environment                   string
	ghAPIBaseURL                  string
	h                             *renderer.Renderer
	kmc                           KeyManagementClient
	runnerExecutionTimeoutSeconds int
	runnerIdleTimeoutSeconds      int
	externalRunnerEndpoint        string
	iapServiceAudience            string
	installationID                int64
	jitConfigAllowlist            map[string]map[string][]string // owner -> repo -> labels
	extraRunnerCount              int
	webhookSecret                 []byte
	e2eTestRunID                  string
	runnerLabel                   string
	enableSelfHostedLabel         bool
	httpClient                    *http.Client
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
	KeyManagementClientOpts []option.ClientOption

	OSFileReaderOverride        FileReader
	KeyManagementClientOverride KeyManagementClient
	HTTPClientOverride          *http.Client
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

	options := []githubauth.Option{
		githubauth.WithBaseURL(cfg.GitHubAPIBaseURL),
	}

	appClient, err := githubauth.NewApp(cfg.GitHubAppID, signer, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to setup app client: %w", err)
	}

	// cfg.Validate() is called before NewServer, safe to convert
	extraRunnerCount, _ := strconv.Atoi(cfg.ExtraRunnerCount)
	runnerIdleTimeoutSeconds, _ := strconv.Atoi(cfg.RunnerIdleTimeoutSeconds)
	runnerExecutionTimeoutSeconds, _ := strconv.Atoi(cfg.RunnerExecutionTimeoutSeconds)

	httpClient := wco.HTTPClientOverride
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	jitConfigAllowlist := make(map[string]map[string][]string)
	if cfg.JITConfigAllowlist != "" {
		if err := json.Unmarshal([]byte(cfg.JITConfigAllowlist), &jitConfigAllowlist); err != nil {
			return nil, fmt.Errorf("failed to parse JIT_CONFIG_ALLOWLIST: %w", err)
		}
	}

	installationID, err := strconv.ParseInt(cfg.GitHubAppInstallationID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GITHUB_APP_INSTALLATION_ID: %w", err)
	}

	return &Server{
		appClient:                     appClient,
		extraRunnerCount:              extraRunnerCount,
		environment:                   cfg.Environment,
		ghAPIBaseURL:                  cfg.GitHubAPIBaseURL,
		h:                             h,
		kmc:                           kmc,
		runnerExecutionTimeoutSeconds: runnerExecutionTimeoutSeconds,
		runnerIdleTimeoutSeconds:      runnerIdleTimeoutSeconds,
		runnerLabel:                   cfg.RunnerLabel,
		externalRunnerEndpoint:        cfg.ExternalRunnerEndpoint,
		iapServiceAudience:            cfg.IAPServiceAudience,
		installationID:                installationID,
		jitConfigAllowlist:            jitConfigAllowlist,
		webhookSecret:                 webhookSecret,
		e2eTestRunID:                  cfg.E2ETestRunID,
		enableSelfHostedLabel:         cfg.EnableSelfHostedLabel,
		httpClient:                    httpClient,
	}, nil
}

// Routes creates a ServeMux for all the routes that
// this Router supports.
func (s *Server) Routes(ctx context.Context) http.Handler {
	logger := logging.FromContext(ctx)
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthcheck.HandleHTTPHealthCheck())
	mux.Handle("POST /webhook", s.handleWebhook())
	mux.Handle("POST /jit-config", s.handleJITConfig())
	mux.Handle("GET /version", s.handleVersion())

	// Middleware
	root := logging.HTTPInterceptor(logger, "")(mux)

	return root
}

// SetAppClient sets the app client for the server.
func (s *Server) SetAppClient(app *githubauth.App) {
	s.appClient = app
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

	return nil
}
