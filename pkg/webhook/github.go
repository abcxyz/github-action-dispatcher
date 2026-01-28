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

package webhook

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

func (s *Server) GenerateRepoJITConfig(ctx context.Context, installationID int64, org, repo, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	return s.generateJITConfig(ctx, installationID, org, &repo, runnerName, runnerLabel)
}

func (s *Server) GenerateOrgJITConfig(ctx context.Context, installationID int64, org, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	return s.generateJITConfig(ctx, installationID, org, nil, runnerName, runnerLabel)
}

// retryableRoundTripper is a custom http.RoundTripper that retries requests with exponential backoff.
type retryableRoundTripper struct {
	transport http.RoundTripper
	maxRetries int
	backoff    time.Duration
}

// RoundTrip implements the http.RoundTripper interface.
func (rt *retryableRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error
	for i := 0; i < rt.maxRetries; i++ {
		resp, err := rt.transport.RoundTrip(req)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		lastErr = err
		if resp != nil {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		time.Sleep(rt.backoff * time.Duration(i+1))
	}
	return nil, fmt.Errorf("failed after %d retries: %w", rt.maxRetries, lastErr)
}

func (s *Server) generateJITConfig(ctx context.Context, installationID int64, org string, repo *string, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	installation, err := s.appClient.InstallationForID(ctx, strconv.FormatInt(installationID, 10))
	if err != nil {
		return nil, fmt.Errorf("failed to setup installation client: %w", err)
	}

	baseTransport := &http.Transport{}
	retryableTransport := &retryableRoundTripper{
		transport: baseTransport,
		maxRetries: 3,
		backoff:    2 * time.Second,
	}

	oauthClient := oauth2.NewClient(ctx, (*installation).AllReposOAuth2TokenSource(ctx, map[string]string{
		"administration": "write",
	}))
	oauthClient.Transport = &oauth2.Transport{
		Base:   retryableTransport,
		Source: oauth2.ReuseTokenSource(nil, (*installation).AllReposOAuth2TokenSource(ctx, map[string]string{
			"administration": "write",
		})),
	}

	gh := github.NewClient(oauthClient)
	baseURL, err := url.Parse(fmt.Sprintf("%s/", s.ghAPIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to set github base URL: %w", err)
	}
	gh.BaseURL = baseURL
	gh.UploadURL = baseURL

	// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
	// guaranteed that particular job will run on this specific runner.
	jitRequest := &github.GenerateJITConfigRequest{
		Name:          runnerName,
		RunnerGroupID: 1,
		Labels:        []string{runnerLabel},
	}

	var jitConfig *github.JITRunnerConfig

	if repo != nil {
		jitConfig, _, err = gh.Actions.GenerateRepoJITConfig(ctx, org, *repo, jitRequest)
	} else {
		jitConfig, _, err = gh.Actions.GenerateOrgJITConfig(ctx, org, jitRequest)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate jitconfig: %w", err)
	}
	return jitConfig, nil
}
