// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-github/v69/github"
	goretry "github.com/sethvargo/go-retry"
	"golang.org/x/oauth2"

	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/logging"
)

// Client is an interface for mocking the GitHub client.
type Client interface {
	GenerateRepoJITConfig(ctx context.Context, installationID int64, org, repo, runnerName, runnerLabel string) (*github.JITRunnerConfig, error)
	GenerateOrgJITConfig(ctx context.Context, installationID int64, org, runnerName, runnerLabel string) (*github.JITRunnerConfig, error)
}

// githubClient implements the Client interface.
type githubClient struct {
	appClient           *githubauth.App
	ghAPIBaseURL        string
	backoffInitialDelay time.Duration
	maxRetryAttempts    int
}

// NewClient creates a new GitHub client.
func NewClient(appClient *githubauth.App, ghAPIBaseURL string, backoffInitialDelay time.Duration, maxRetryAttempts int) Client {
	return &githubClient{
		appClient:           appClient,
		ghAPIBaseURL:        ghAPIBaseURL,
		backoffInitialDelay: backoffInitialDelay,
		maxRetryAttempts:    maxRetryAttempts,
	}
}

// GenerateRepoJITConfig creates a JIT config for a repository-level runner.
func (g *githubClient) GenerateRepoJITConfig(ctx context.Context, installationID int64, org, repo, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	return g.generateJITConfig(ctx, installationID, org, &repo, runnerName, runnerLabel)
}

// GenerateOrgJITConfig creates a JIT config for an organization-level runner.
func (g *githubClient) GenerateOrgJITConfig(ctx context.Context, installationID int64, org, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	return g.generateJITConfig(ctx, installationID, org, nil, runnerName, runnerLabel)
}

func (g *githubClient) generateJITConfig(ctx context.Context, installationID int64, org string, repo *string, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	logger := logging.FromContext(ctx)

	installation, err := g.appClient.InstallationForID(ctx, fmt.Sprintf("%d", installationID))
	if err != nil {
		return nil, fmt.Errorf("failed to setup installation client: %w", err)
	}

	oauthTransport := &oauth2.Transport{
		Base: http.DefaultTransport,
		Source: oauth2.ReuseTokenSource(nil, (*installation).AllReposOAuth2TokenSource(ctx, map[string]string{
			"administration": "write",
		})),
	}

	httpClient := &http.Client{
		Transport: oauthTransport,
	}

	gh := github.NewClient(httpClient)

	baseURL, err := url.Parse(fmt.Sprintf("%s/", g.ghAPIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to set github base URL: %w", err)
	}
	gh.BaseURL = baseURL
	gh.UploadURL = baseURL

	jitRequest := &github.GenerateJITConfigRequest{
		Name:          runnerName,
		RunnerGroupID: 1,
		Labels:        []string{runnerLabel},
	}

	var jitConfig *github.JITRunnerConfig
	backoff := g.newBackoff()

	if err := goretry.Do(ctx, backoff, func(ctx context.Context) error {
		var resp *github.Response
		var err error
		if repo != nil {
			jitConfig, resp, err = gh.Actions.GenerateRepoJITConfig(ctx, org, *repo, jitRequest)
		} else {
			jitConfig, resp, err = gh.Actions.GenerateOrgJITConfig(ctx, org, jitRequest)
		}

		if err != nil {
			logger.WarnContext(ctx, "retrying due to GitHub API call failure", "error", err)
			// Network errors or other client errors from go-github itself are retryable.
			return goretry.RetryableError(fmt.Errorf("GitHub API call failed: %w", err))
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Success, stop retrying.
			return nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			logger.WarnContext(ctx, "retrying due to server error", "status_code", resp.StatusCode)
			// Retry on 429 and 5xx errors.
			return goretry.RetryableError(fmt.Errorf("server responded with %d status code", resp.StatusCode))
		}

		// Other 4xx errors are not retryable. Propagate immediately.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("server responded with non-retryable client error: %d", resp.StatusCode)
		}

		// Fallback for unexpected status codes, treat as non-retryable.
		return fmt.Errorf("server responded with unexpected status code: %d", resp.StatusCode)
	}); err != nil {
		return nil, fmt.Errorf("failed to generate jitconfig after retries: %w", err)
	}

	return jitConfig, nil
}

func (g *githubClient) newBackoff() goretry.Backoff {
	backoff := goretry.NewExponential(g.backoffInitialDelay)
	if g.maxRetryAttempts >= 0 {
		backoff = goretry.WithMaxRetries(uint64(g.maxRetryAttempts), backoff)
	}
	return backoff
}
