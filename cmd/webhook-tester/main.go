// Copyright 2024 The Authors (see AUTHORS file)
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
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

var (
	webhookURL     = flag.String("webhook-url", "", "The URL of the webhook to test.")
	secretName     = flag.String("secret-name", "", "The name of the secret in Secret Manager.")
	installationID = flag.Int64("installation-id", 0, "The ID of the GitHub App installation.")
	projectID      = flag.String("project-id", "", "The GCP project ID for the integration environment.")
	runID          = flag.String("run-id", "", "The unique GitHub Actions workflow run ID.")
	idToken        = flag.String("id-token", "", "The ID token for authenticating to the webhook service.")
	verifyRunner   = flag.Bool("verify-runner", false, "If true, verify the runner is online instead of the build.")
	githubOwner    = flag.String("github-owner", "", "The GitHub owner (organization).")
	githubRepo     = flag.String("github-repo", "", "The GitHub repository name.")
)

const (
	// validPayloadTemplate is a template for a valid webhook payload.
	// The installation ID and workflow job ID are injected dynamically.
	validPayloadTemplate = `{
		"action": "queued",
		"workflow_job": {
			"id": %d,
			"run_id": %d,
			"name": "test-job",
			"labels": ["self-hosted"],
			"created_at": "2024-01-01T00:00:00Z"
		},
		"organization": {
			"login": "abcxyz"
		},
		"repository": {
			"name": "github-action-dispatcher-testing"
		},
		"installation": {
			"id": %d
		}
	}`
)

type runnerInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type runnersResponse struct {
	Runners []runnerInfo `json:"runners"`
}

func main() {
	flag.Parse()

	if *webhookURL == "" {
		log.Fatal("--webhook-url is required.")
	}
	if *secretName == "" {
		log.Fatal("--secret-name is required.")
	}
	if *installationID == 0 {
		log.Fatal("--installation-id is required.")
	}
	if *projectID == "" {
		log.Fatal("--project-id is required.")
	}
	if *runID == "" {
		log.Fatal("--run-id is required.")
	}
	if *verifyRunner {
		if *githubOwner == "" {
			log.Fatal("--github-owner is required with --verify-runner.")
		}
		if *githubRepo == "" {
			log.Fatal("--github-repo is required with --verify-runner.")
		}
	}

	ctx := context.Background()
	secret, err := getSecret(ctx, *secretName)
	if err != nil {
		log.Fatalf("failed to get secret: %v", err)
	}

	runIDInt, err := strconv.ParseInt(*runID, 10, 64)
	if err != nil {
		log.Fatalf("failed to parse run-id: %v", err)
	}

	body := fmt.Sprintf(validPayloadTemplate, runIDInt, runIDInt, *installationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *webhookURL, bytes.NewBuffer([]byte(body)))
	if err != nil {
		log.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", "sha256="+signature([]byte(secret), []byte(body)))
	if *idToken != "" {
		req.Header.Set("Authorization", "Bearer "+*idToken)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("failed to read response body: %v", err)
	}

	log.Printf("Status: %s", resp.Status)
	log.Printf("Body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}

	log.Printf("Successfully sent webhook payload. Now verifying...")

	if *verifyRunner {
		if err := verifyRunnerOnline(ctx, *githubOwner, *githubRepo, *runID); err != nil {
			log.Fatalf("Failed to verify runner: %v", err)
		}
		log.Printf("Successfully verified runner is online.")
	} else {
		if err := verifyBuildTriggered(ctx, *projectID, *runID); err != nil {
			log.Fatalf("Failed to verify build trigger: %v", err)
		}
		log.Printf("Successfully verified build trigger.")
	}
}

// verifyBuildTriggered polls gcloud to check if a build was triggered with the correct tag.
func verifyBuildTriggered(ctx context.Context, projectID, runID string) error {
	const (
		maxRetries = 10
		delay      = 30 * time.Second
	)

	filter := fmt.Sprintf("tags='e2e-run-id-%s'", runID)

	for i := 0; i < maxRetries; i++ {
		log.Printf("Polling for build (attempt %d/%d)...", i+1, maxRetries)
		cmd := exec.CommandContext(ctx, "gcloud", "builds", "list", "--project", projectID, "--filter", filter, "--format", "value(id)")
		output, err := cmd.Output()
		if err != nil {
			// gcloud might write to stderr, so let's capture that too for better debugging
			if ee, ok := err.(*exec.ExitError); ok {
				return fmt.Errorf("gcloud command failed: %w, stderr: %s", err, string(ee.Stderr))
			}
			return fmt.Errorf("gcloud command failed: %w", err)
		}

		buildID := strings.TrimSpace(string(output))
		if buildID != "" {
			log.Printf("Found build ID: %s", buildID)
			return nil
		}

		time.Sleep(delay)
	}

	return fmt.Errorf("timed out waiting for build to be triggered")
}

// verifyRunnerOnline polls the GitHub API to check if a runner with the correct name is online.
func verifyRunnerOnline(ctx context.Context, owner, repo, runID string) error {
	const (
		maxRetries = 10
		delay      = 30 * time.Second
	)

	runnerName := fmt.Sprintf("GCP-%s", runID)

	for i := 0; i < maxRetries; i++ {
		log.Printf("Polling for runner (attempt %d/%d)...", i+1, maxRetries)
		cmd := exec.CommandContext(ctx, "gh", "api", "--paginate", fmt.Sprintf("repos/%s/%s/actions/runners", owner, repo))
		output, err := cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return fmt.Errorf("gh command failed: %w, stderr: %s", err, string(ee.Stderr))
			}
			return fmt.Errorf("gh command failed: %w", err)
		}

		var response runnersResponse
		if err := json.Unmarshal(output, &response); err != nil {
			return fmt.Errorf("failed to unmarshal runner list: %w", err)
		}

		for _, runner := range response.Runners {
			if runner.Name == runnerName && runner.Status == "online" {
				log.Printf("Found runner %s with status %s", runner.Name, runner.Status)
				return nil
			}
		}

		time.Sleep(delay)
	}

	return fmt.Errorf("timed out waiting for runner to be online")
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

	return string(result.Payload.Data), nil
}

// signature calculates the signature for the given secret and body.
func signature(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
