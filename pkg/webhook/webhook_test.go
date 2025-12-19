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
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v69/github"

	"github.com/abcxyz/pkg/githubauth"
)

const (
	SHA256SignatureHeader = "X-Hub-Signature-256"
	EventTypeHeader       = "X-Github-Event"
	DeliveryIDHeader      = "X-Github-Delivery"
	ContentTypeHeader     = "Content-Type"
	//nolint:gosec // this is a test value
	serverGitHubWebhookSecret = "test-github-webhook-secret"
)

func TestHandleWebhook(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	queuedTime := github.Timestamp{Time: now.Add(-15 * time.Minute)}
	inProgressTime := github.Timestamp{Time: now.Add(-10 * time.Minute)}
	completedTime := github.Timestamp{Time: now.Add(-5 * time.Minute)}
	runID := int64(456)
	jobID := int64(789)
	jobName := "build-job"

	queuedAction := "queued"
	contentType := "application/json"
	payloadType := "workflow_job"

	cases := []struct {
		name                          string
		payloadType                   string
		action                        string
		runnerExecutionTimeoutSeconds int
		runnerLabels                  []string
		payloadWebhookSecret          string
		serverRunnerLabel             string
		ServerEnableSelfHostedLabel   bool
		extraSpawnNumber              int
		contentType                   string
		createdAt                     *github.Timestamp
		startedAt                     *github.Timestamp
		completedAt                   *github.Timestamp
		runID                         *int64
		jobID                         *int64
		jobName                       *string
		expStatusCode                 int
		expRespBody                   string // This is now only for plain text responses.
		expectBuildCount              int
	}{
		{
			name:                          "Workflow Job Queued - Default Label",
			payloadType:                   payloadType,
			action:                        queuedAction,
			runnerExecutionTimeoutSeconds: 7200,
			runnerLabels:                  []string{deprecatedSelfHostedRunnerLabel},
			payloadWebhookSecret:          serverGitHubWebhookSecret,
			serverRunnerLabel:             "self-hosted",
			contentType:                   contentType,
			createdAt:                     &queuedTime,
			startedAt:                     nil,
			completedAt:                   nil,
			runID:                         &runID,
			jobID:                         &jobID,
			jobName:                       &jobName,
			expStatusCode:                 200,
			expRespBody:                   "",
			expectBuildCount:              1,
		},
		{
			name:                 "Workflow Job Queued - Custom Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"custom-label"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			serverRunnerLabel:    "custom-label",
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "",
			expectBuildCount:     1,
		},
		{
			name:                        "Workflow Job Queued - Default And Custom Label",
			payloadType:                 payloadType,
			action:                      queuedAction,
			runnerLabels:                []string{"self-hosted"},
			payloadWebhookSecret:        serverGitHubWebhookSecret,
			serverRunnerLabel:           "custom-label",
			ServerEnableSelfHostedLabel: true,
			contentType:                 contentType,
			createdAt:                   &queuedTime,
			startedAt:                   nil,
			completedAt:                 nil,
			runID:                       &runID,
			jobID:                       &jobID,
			jobName:                     &jobName,
			expStatusCode:               200,
			expRespBody:                 "",
			expectBuildCount:            1,
		},
		{
			name:                 "Workflow Job Queued - Multiple Builds Spawned",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{deprecatedSelfHostedRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			serverRunnerLabel:    "self-hosted",
			extraSpawnNumber:     2,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "",
			expectBuildCount:     3,
		},
		{
			name:                        "Workflow Job Queued - Multiple Label Fails",
			payloadType:                 payloadType,
			action:                      queuedAction,
			runnerLabels:                []string{"self-hosted", "custom-label"},
			payloadWebhookSecret:        serverGitHubWebhookSecret,
			serverRunnerLabel:           "custom-label",
			ServerEnableSelfHostedLabel: true,
			contentType:                 contentType,
			createdAt:                   &queuedTime,
			startedAt:                   nil,
			completedAt:                 nil,
			runID:                       &runID,
			jobID:                       &jobID,
			jobName:                     &jobName,
			expStatusCode:               200,
			expRespBody:                 fmt.Sprintf("no action taken, only accept single label jobs, got: %s", []string{"self-hosted", "custom-label"}),
			expectBuildCount:            0,
		},
		{
			name:                 "Workflow Job Queued - No Matching Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"other-label"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			serverRunnerLabel:    "self-hosted",
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          fmt.Sprintf("no action taken for label: %s", []string{"other-label"}),
			expectBuildCount:     0,
		},
		{
			name:                 "Workflow Job In Progress",
			payloadType:          payloadType,
			action:               "in_progress",
			runnerLabels:         []string{deprecatedSelfHostedRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			serverRunnerLabel:    "self-hosted",
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            &inProgressTime,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "workflow job in progress event logged",
			expectBuildCount:     0,
		},
		{
			name:                 "Workflow Job Completed - Success",
			payloadType:          payloadType,
			action:               "completed",
			runnerLabels:         []string{deprecatedSelfHostedRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			serverRunnerLabel:    "self-hosted",
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            &inProgressTime,
			completedAt:          &completedTime,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "workflow job completed event logged",
			expectBuildCount:     0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Mock External Runner Endpoint
			var runnerRequests []*runnerRequest
			externalEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				var req runnerRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				runnerRequests = append(runnerRequests, &req)
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "OK")
			}))
			defer externalEndpoint.Close()

			orgLogin := "google"
			repoName := "webhook"
			installationID := int64(123)
			event := &github.WorkflowJobEvent{
				Action: &tc.action,
				WorkflowJob: &github.WorkflowJob{
					Labels:      tc.runnerLabels,
					CreatedAt:   tc.createdAt,
					StartedAt:   tc.startedAt,
					CompletedAt: tc.completedAt,
					RunID:       tc.runID,
					ID:          tc.jobID,
					Name:        tc.jobName,
				},
				Installation: &github.Installation{
					ID: &installationID,
				},
				Org: &github.Organization{
					Login: &orgLogin,
				},
				Repo: &github.Repository{
					Name: &repoName,
				},
			}

			payload, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}

			encodedJitConfig := "Hello"
			jit := &github.JITRunnerConfig{
				EncodedJITConfig: &encodedJitConfig,
			}
			jitPayload, err := json.Marshal(jit)
			if err != nil {
				t.Fatal(err)
			}

			fakeGitHub := func() *httptest.Server {
				mux := http.NewServeMux()
				mux.Handle("GET /app/installations/123", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprintf(w, `{"access_tokens_url": "http://%s/app/installations/123/access_tokens"}`, r.Host)
				}))
				mux.Handle("POST /app/installations/123/access_tokens", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(201)
					fmt.Fprintf(w, `{"token": "this-is-the-token-from-github"}`)
				}))
				mux.Handle("POST /repos/google/webhook/actions/runners/generate-jitconfig", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(201)
					fmt.Fprintf(w, "%s", string(jitPayload))
				}))

				return httptest.NewServer(mux)
			}()
			t.Cleanup(func() {
				fakeGitHub.Close()
			})

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
			req.Header.Add(DeliveryIDHeader, "delivery-id")
			req.Header.Add(EventTypeHeader, tc.payloadType)
			req.Header.Add(ContentTypeHeader, tc.contentType)
			req.Header.Add(SHA256SignatureHeader, fmt.Sprintf("sha256=%s", createSignature([]byte(tc.payloadWebhookSecret), payload)))

			resp := httptest.NewRecorder()

			rsaPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}

			app, err := githubauth.NewApp("app-id", rsaPrivateKey, githubauth.WithBaseURL(fakeGitHub.URL))
			if err != nil {
				t.Fatal(err)
			}

			srv := &Server{
				webhookSecret:                 []byte(tc.payloadWebhookSecret),
				appClient:                     app,
				enableSelfHostedLabel:         tc.ServerEnableSelfHostedLabel,
				environment:                   "test",
				extraRunnerCount:              tc.extraSpawnNumber,
				ghAPIBaseURL:                  fakeGitHub.URL,
				runnerExecutionTimeoutSeconds: tc.runnerExecutionTimeoutSeconds,
				runnerLabel:                   tc.serverRunnerLabel,
				externalRunnerEndpoint:        externalEndpoint.URL,
				httpClient:                    http.DefaultClient,
				installationID:                123,
			}
			srv.handleWebhook().ServeHTTP(resp, req)

			if got, want := resp.Code, tc.expStatusCode; got != want {
				t.Errorf("expected %d to be %d", got, want)
			}

			// If the action was "queued" and we expected a build, we check for the JSON response.
			if tc.action == "queued" && tc.expectBuildCount > 0 {
				var r runnersResponse
				if err := json.Unmarshal(resp.Body.Bytes(), &r); err != nil {
					t.Fatalf("failed to unmarshal JSON response: %v, body: %s", err, resp.Body.String())
				}

				if got, want := r.Message, runnerStartedMsg; got != want {
					t.Errorf("expected message %q, got %q", want, got)
				}

				if got, want := len(r.RunnerNames), tc.expectBuildCount; got != want {
					t.Errorf("expected %d runner names in response, but got %d", want, got)
				}
			} else {
				if got, want := strings.TrimSpace(resp.Body.String()), tc.expRespBody; got != want {
					t.Errorf("expected %q to be %q", got, want)
				}
			}

			if got, want := len(runnerRequests), tc.expectBuildCount; got != want {
				t.Errorf("expected %d runner request(s) to be sent, but got %d", want, got)
			}
			// check labels of requests
			for _, r := range runnerRequests {
				if r.Label == "" {
					t.Error("expected runner request to have a label")
				}
			}
		})
	}
}

func TestHandleJITConfig(t *testing.T) {
	// Not safe to run parallel because we modify global validateIAPToken
	// t.Parallel()

	// Backup and Restore validateIAPToken
	originalValidator := validateIAPToken
	t.Cleanup(func() {
		validateIAPToken = originalValidator
	})

	validToken := "valid-token"
	validAudience := "valid-audience"

	validateIAPToken = func(ctx context.Context, token, audience string) error {
		if token == "" {
			return fmt.Errorf("missing IAP token")
		}
		if token == validToken && audience == validAudience {
			return nil
		}
		return fmt.Errorf("invalid token")
	}

	cases := []struct {
		name          string
		token         string
		audience      string
		allowlist     map[string]map[string][]string
		payload       jitConfigPayload
		mockGitHubJIT bool // Should we mock GitHub JIT config generation?
		expStatusCode int
		expRespBody   string
	}{
		{
			name:          "Missing IAP Token",
			token:         "",
			audience:      validAudience,
			allowlist:     map[string]map[string][]string{"owner": {"repo": {"label"}}},
			payload:       jitConfigPayload{Owner: "owner", Repo: "repo", Labels: []string{"label"}},
			expStatusCode: http.StatusForbidden,
			expRespBody:   "invalid IAP token",
		},
		{
			name:          "Invalid IAP Token",
			token:         "invalid",
			audience:      validAudience,
			allowlist:     map[string]map[string][]string{"owner": {"repo": {"label"}}},
			payload:       jitConfigPayload{Owner: "owner", Repo: "repo", Labels: []string{"label"}},
			expStatusCode: http.StatusForbidden,
			expRespBody:   "invalid IAP token",
		},
		{
			name:          "Denied by Allowlist",
			token:         validToken,
			audience:      validAudience,
			allowlist:     map[string]map[string][]string{"owner": {"repo": {"other-label"}}},
			payload:       jitConfigPayload{Owner: "owner", Repo: "repo", Labels: []string{"label"}},
			expStatusCode: http.StatusForbidden,
			expRespBody:   "request denied by allowlist",
		},
		{
			name:          "Success",
			token:         validToken,
			audience:      validAudience,
			allowlist:     map[string]map[string][]string{"owner": {"repo": {"label"}}},
			payload:       jitConfigPayload{Owner: "owner", Repo: "repo", Labels: []string{"label"}},
			mockGitHubJIT: true,
			expStatusCode: http.StatusOK,
			expRespBody:   "", // check JSON
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeGitHub := func() *httptest.Server {
				mux := http.NewServeMux()
				mux.Handle("GET /repos/owner/repo/installation", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
					fmt.Fprintf(w, `{"id": 123}`)
				}))
				mux.Handle("GET /app/installations/123", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprintf(w, `{"access_tokens_url": "http://%s/app/installations/123/access_tokens"}`, r.Host)
				}))
				mux.Handle("POST /app/installations/123/access_tokens", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(201)
					fmt.Fprintf(w, `{"token": "this-is-the-token-from-github"}`)
				}))
				mux.Handle("POST /repos/owner/repo/actions/runners/generate-jitconfig", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !tc.mockGitHubJIT {
						w.WriteHeader(500)
						return
					}
					// check payload labels?
					encoded := "jit"
					resp := github.JITRunnerConfig{EncodedJITConfig: &encoded}
					json.NewEncoder(w).Encode(resp)
				}))
				return httptest.NewServer(mux)
			}()
			defer fakeGitHub.Close()

			rsaPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}
			app, err := githubauth.NewApp("app-id", rsaPrivateKey, githubauth.WithBaseURL(fakeGitHub.URL))
			if err != nil {
				t.Fatal(err)
			}

			srv := &Server{
				iapServiceAudience: validAudience,
				jitConfigAllowlist: tc.allowlist,
				appClient:          app,
				ghAPIBaseURL:       fakeGitHub.URL,
				installationID:     123,
			}

			payloadBytes, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/jit-config", bytes.NewReader(payloadBytes))
			if tc.token != "" {
				req.Header.Set("x-goog-iap-jwt-assertion", tc.token)
			}

			resp := httptest.NewRecorder()
			srv.handleJITConfig().ServeHTTP(resp, req)

			if got, want := resp.Code, tc.expStatusCode; got != want {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected status code %d, got %d. Body: %s", want, got, string(body))
			}

			if tc.expRespBody != "" {
				if !strings.Contains(resp.Body.String(), tc.expRespBody) {
					t.Errorf("expected response to contain %q, got %q", tc.expRespBody, resp.Body.String())
				}
			}
		})
	}
}

func createSignature(key, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
