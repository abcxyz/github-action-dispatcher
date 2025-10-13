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

// webhook-tester is a tool for testing the webhook locally.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"

	"github.com/abcxyz/pkg/logging"
)

var (
	webhookURL       = flag.String("webhook-url", "", "The URL of the webhook to test.")
	secretName       = flag.String("secret-name", "", "The name of the secret in Secret Manager.")
	installationID   = flag.Int64("installation-id", 0, "The ID of the GitHub App installation.")
	projectID        = flag.String("project-id", "", "The GCP project ID for the integration environment.")
	runID            = flag.String("run-id", "", "The unique GitHub Actions workflow run ID.")
	idToken          = flag.String("id-token", "", "The ID token for authenticating to the webhook service.")
	verifyRunner     = flag.Bool("verify-runner", false, "If true, verify the runner is online instead of the build.")
	githubOwner      = flag.String("github-owner", "", "The GitHub owner (organization).")
	githubRepo       = flag.String("github-repo", "", "The GitHub repository name.")
	githubToken      = flag.String("github-token", "", "The GitHub token for authenticating to the API.")
	signatureFlag    = flag.String("signature", "", "The signature to use for the webhook payload. If empty, it will be calculated.")
	expectHTTPStatus = flag.Int("expect-http-status", http.StatusOK, "The expected HTTP status code.")
	payload          = flag.String("payload", "", "The payload to send. If empty, a default valid payload is used.")
	payloadFile      = flag.String("payload-file", "", "The path to a file containing the payload to send.")
	expectNoBuild    = flag.Bool("expect-no-build", false, "If true, verify that no build was triggered.")
)

type runnersResponse struct {
	Message     string   `json:"message"`
	RunnerNames []string `json:"runnerNames"`
}

func main() {
	ctx, done := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer done()

	logger := logging.NewFromEnv("")
	ctx = logging.WithLogger(ctx, logger)

	if err := realMain(ctx); err != nil {
		done()
		logger.ErrorContext(ctx, "process exited with error", "error", err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context) error {
	flag.Parse()

	if *webhookURL == "" {
		return fmt.Errorf("--webhook-url is required")
	}
	if *secretName == "" {
		return fmt.Errorf("--secret-name is required")
	}
	if *installationID == 0 {
		return fmt.Errorf("--installation-id is required")
	}
	if *projectID == "" {
		return fmt.Errorf("--project-id is required")
	}
	if *runID == "" {
		return fmt.Errorf("--run-id is required")
	}
	if *verifyRunner {
		if *githubOwner == "" {
			return fmt.Errorf("--github-owner is required with --verify-runner")
		}
		if *githubRepo == "" {
			return fmt.Errorf("--github-repo is required with --verify-runner")
		}
	}

	if *payload == "" && *payloadFile == "" {
		return fmt.Errorf("either --payload or --payload-file is required")
	}

	secret, err := getSecret(ctx, *secretName)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	body := *payload
	if *payloadFile != "" {
		payloadBytes, err := os.ReadFile(*payloadFile)
		if err != nil {
			return fmt.Errorf("failed to read payload file: %w", err)
		}
		body = string(payloadBytes)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *webhookURL, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	sig := *signatureFlag
	if sig == "" {
		sig = signature([]byte(secret), []byte(body))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
	if *idToken != "" {
		req.Header.Set("Authorization", "Bearer "+*idToken)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Limit the response body to 4MB to prevent reading excessively large
	// responses.
	limitReader := &io.LimitedReader{R: resp.Body, N: 4_194_304}
	respBody, err := io.ReadAll(limitReader)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	log.Printf("Status: %s", resp.Status)
	log.Printf("Body: %s", string(respBody))

	if resp.StatusCode != *expectHTTPStatus {
		return fmt.Errorf("expected status %d, but got %d", *expectHTTPStatus, resp.StatusCode)
	}

	log.Printf("Successfully received expected status code %d.", *expectHTTPStatus)

	var runnerNames []string
	if *verifyRunner && *expectHTTPStatus == http.StatusOK {
		var r runnersResponse
		if err := json.Unmarshal(respBody, &r); err != nil {
			return fmt.Errorf("failed to parse JSON response from webhook: %w", err)
		}
		if len(r.RunnerNames) == 0 {
			return fmt.Errorf("webhook response did not include any runner names")
		}
		runnerNames = r.RunnerNames
		log.Printf("Extracted runner names from response: %v", runnerNames)
	}

	cloudBuildClient, err := cloudbuild.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create cloudbuild client: %w", err)
	}
	defer cloudBuildClient.Close()

	if *expectNoBuild {
		if err := verifyNoBuildTriggered(ctx, cloudBuildClient, *projectID, *runID); err != nil {
			return fmt.Errorf("failed to verify that no build was triggered: %w", err)
		}
		log.Printf("Successfully verified that no build was triggered.")
		return nil
	}

	// Only run verification if the test was expected to succeed.
	if *expectHTTPStatus == http.StatusOK {
		log.Printf("Successfully sent webhook payload. Now verifying...")

		if *verifyRunner {
			if err := verifyRunnerOnline(ctx, *githubOwner, *githubRepo, runnerNames); err != nil {
				return fmt.Errorf("failed to verify runner: %w", err)
			}
			log.Printf("Successfully verified runner is online.")
		} else {
			if err := verifyBuildTriggered(ctx, cloudBuildClient, *projectID, *runID); err != nil {
				return fmt.Errorf("failed to verify build trigger: %w", err)
			}
			log.Printf("Successfully verified build trigger.")
		}
	}
	return nil
}

// verifyBuildTriggered polls GCP to check if a build was triggered with the correct tag.
func verifyBuildTriggered(ctx context.Context, client *cloudbuild.Client, projectID, runID string) error {
	const (
		maxRetries = 10
		delay      = 30 * time.Second
	)

	filter := fmt.Sprintf(`tags="e2e-run-id-%s"`, runID)

	for i := 0; i < maxRetries; i++ {
		log.Printf("Polling for build (attempt %d/%d)...", i+1, maxRetries)

		it := client.ListBuilds(ctx, &cloudbuildpb.ListBuildsRequest{
			ProjectId: projectID,
			Filter:    filter,
		})
		for {
			build, err := it.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to list builds: %w", err)
			}

			if build != nil {
				log.Printf("Found build ID: %s", build.GetId())
				return nil
			}
		}

		time.Sleep(delay)
	}

	return fmt.Errorf("timed out waiting for build to be triggered")
}

// verifyNoBuildTriggered polls GCP to ensure that no build was triggered with the correct tag.
func verifyNoBuildTriggered(ctx context.Context, client *cloudbuild.Client, projectID, runID string) error {
	const (
		retries = 3
		delay   = 10 * time.Second
	)

	filter := fmt.Sprintf(`tags="e2e-run-id-%s"`, runID)

	for i := 0; i < retries; i++ {
		log.Printf("Polling to ensure no build was triggered (attempt %d/%d)...", i+1, retries)
		it := client.ListBuilds(ctx, &cloudbuildpb.ListBuildsRequest{
			ProjectId: projectID,
			Filter:    filter,
		})
		for {
			build, err := it.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to list builds: %w", err)
			}

			if build != nil {
				return fmt.Errorf("found build ID %s, but no build was expected", build.GetId())
			}
		}

		time.Sleep(delay)
	}

	return nil
}

// verifyRunnerOnline polls the GitHub API to check if a runner with the correct name is online.
func verifyRunnerOnline(ctx context.Context, owner, repo string, expectedRunnerNames []string) error {
	const (
		maxRetries = 10
		delay      = 30 * time.Second
	)

	// Create a map to track which runners we have found.
	foundRunners := make(map[string]bool)
	for _, name := range expectedRunnerNames {
		foundRunners[name] = false
	}

	var httpClient *http.Client
	if *githubToken != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: *githubToken})
		httpClient = oauth2.NewClient(ctx, ts)
	}
	client := github.NewClient(httpClient)

	for i := 0; i < maxRetries; i++ {
		log.Printf("Polling for runners (attempt %d/%d)...", i+1, maxRetries)

		opts := &github.ListRunnersOptions{}
		runners, _, err := client.Actions.ListRunners(ctx, owner, repo, opts)
		if err != nil {
			return fmt.Errorf("failed to list runners: %w", err)
		}

		// Check the API results against our map of expected runners.
		for _, runner := range runners.Runners {
			if _, ok := foundRunners[runner.GetName()]; ok && runner.GetStatus() == "online" {
				log.Printf("Found expected runner %s with status %s", runner.GetName(), runner.GetStatus())
				foundRunners[runner.GetName()] = true
			}
		}

		// Check if all expected runners have been found.
		allFound := true
		for _, found := range foundRunners {
			if !found {
				allFound = false
				break
			}
		}

		if allFound {
			log.Printf("Successfully found all expected runners.")
			return nil
		}

		time.Sleep(delay)
	}

	var missingRunners []string
	for name, found := range foundRunners {
		if !found {
			missingRunners = append(missingRunners, name)
		}
	}
	return fmt.Errorf("timed out waiting for runners to be online, missing: %v", missingRunners)
}

// getSecret gets the secret from Secret Manager.
func getSecret(ctx context.Context, name string) (string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create secret manager client: %w", err)
	}
	defer client.Close()

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	result, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to access secret version: %w", err)
	}

	return string(result.GetPayload().GetData()), nil
}

// signature calculates the signature for the given secret and body.
func signature(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
