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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v69/github"

	"github.com/abcxyz/github-action-dispatcher/pkg/cloudbuild"
	gh "github.com/abcxyz/github-action-dispatcher/pkg/github"
	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/pkg/logging"
)

const (
	SHA256SignatureHeader             = "X-Hub-Signature-256"
	EventTypeHeader                   = "X-Github-Event"
	DeliveryIDHeader                  = "X-Github-Delivery"
	ContentTypeHeader                 = "Content-Type"
	SelfHostedRunnerLabel             = "self-hosted"
	SelfHostedUbuntuLatestRunnerLabel = "sh-ubuntu-latest"
	testGCBBuildID                    = "test-build-id"
	//nolint:gosec // this is a test value
	serverGitHubWebhookSecret = "test-github-webhook-secret"
	testEnv                   = "test"
	// Webhook event details.
	orgLogin = "google"
	repoName = "webhook"
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
		name                 string
		payloadType          string
		action               string
		runnerLabels         []string
		payloadWebhookSecret string
		extraSpawnNumber     int
		contentType          string
		createdAt            *github.Timestamp
		startedAt            *github.Timestamp
		completedAt          *github.Timestamp
		runID                *int64
		jobID                *int64
		jobName              *string
		expStatusCode        int
		expRespBody          string // This is now only for plain text responses.
		expectBuildCount     int
		expGCBBuildIDs       []string
		exp404               bool

		runnerExecutionTimeoutSeconds  int
		runnerIdleTimeoutSeconds       int
		runnerLabelAliases             map[string]string
		supportedRunnerLabels          []string
		runnerRegistryDefaultKeyPrefix string
		registryWorkerPools            map[string][]registry.WorkerPoolInfo
	}{
		{
			name:                 "Workflow Job Queued - Default Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{SelfHostedRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{SelfHostedRunnerLabel},

			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"google:self-hosted": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Custom Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"custom-label"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{"custom-label"},

			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"google:custom-label": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Aliased Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"old-label"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			runnerLabelAliases: map[string]string{
				"old-label": "new-label",
			},
			supportedRunnerLabels: []string{"new-label"},

			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"google:new-label": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Multiple Builds Spawned",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{SelfHostedRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID, testGCBBuildID, testGCBBuildID},

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{SelfHostedRunnerLabel},
			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"google:self-hosted": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Multiple Label Fails",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"self-hosted", "custom-label"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "no action taken, only accept single label jobs, got: [self-hosted custom-label]",
			expectBuildCount:     0,
		},
		{
			name:                 "Workflow Job Queued - No Matching Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"other-label"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "workflow job completed event logged",
			expectBuildCount:     1,
			exp404:               true,
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{SelfHostedRunnerLabel},
		},
		{
			name:                 "Workflow Job In Progress",
			payloadType:          payloadType,
			action:               "in_progress",
			runnerLabels:         []string{SelfHostedRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            &inProgressTime,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "workflow job in progress event logged",
			expectBuildCount:     0,

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{SelfHostedRunnerLabel},
		},
		{
			name:                 "Workflow Job Completed - Success",
			payloadType:          payloadType,
			action:               "completed",
			runnerLabels:         []string{SelfHostedRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{SelfHostedRunnerLabel},
		},
		{
			name:                 "Workflow Job Queued - Self Hosted Ubuntu Latest",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{SelfHostedUbuntuLatestRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{SelfHostedUbuntuLatestRunnerLabel},

			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"google:sh-ubuntu-latest": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Supported Runner Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"ubuntu-20.04-n2d-standard-2"}, // Directly from SupportedRunnerLabels
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds: 7200,
			runnerIdleTimeoutSeconds:      300,
			supportedRunnerLabels:         []string{"ubuntu-20.04-n2d-standard-2"},

			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"google:ubuntu-20.04-n2d-standard-2": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Worker Pool From Registry",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"ubuntu-20.04-n2d-standard-2"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds:  7200,
			runnerIdleTimeoutSeconds:       300,
			supportedRunnerLabels:          []string{"ubuntu-20.04-n2d-standard-2"},
			runnerRegistryDefaultKeyPrefix: "runner",
			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"runner:ubuntu-20.04-n2d-standard-2": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Worker Pool From Registry - Multiple Pools",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"ubuntu-20.04-n2d-standard-2"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds:  7200,
			runnerIdleTimeoutSeconds:       300,
			supportedRunnerLabels:          []string{"ubuntu-20.04-n2d-standard-2"},
			runnerRegistryDefaultKeyPrefix: "runner",
			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"runner:ubuntu-20.04-n2d-standard-2": {
					{Name: "projects/12345-test-project-1/locations/us-west1/workerPools/wp1", ProjectID: "test-project-1", ProjectNumber: "12345-test-project-1"},
					{Name: "projects/67890-test-project-2/locations/us-west1/workerPools/wp2", ProjectID: "test-project-2", ProjectNumber: "67890-test-project-2"},
				},
			},
		},
		{
			name:                 "Workflow Job Queued - Trusted Pool With Remote Config",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"trusted-label"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
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
			expGCBBuildIDs:       []string{testGCBBuildID},

			runnerExecutionTimeoutSeconds:  7200,
			runnerIdleTimeoutSeconds:       300,
			supportedRunnerLabels:          []string{"trusted-label"},
			runnerRegistryDefaultKeyPrefix: "runner",
			registryWorkerPools: map[string][]registry.WorkerPoolInfo{
				"runner:trusted-label": {
					{
						Name:          "projects/12345-test-project-1/locations/us-west1/workerPools/wp1",
						ProjectID:     "test-project-1",
						ProjectNumber: "12345-test-project-1",
						PoolType:      "trusted",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(t.Context(), logging.TestLogger(t))

			buildTimeoutForTest := tc.runnerExecutionTimeoutSeconds
			if buildTimeoutForTest == 0 {
				buildTimeoutForTest = 3600
			}
			expectedBuildTimeout := time.Duration(buildTimeoutForTest) * time.Second

			installationID := int64(123)
			orgLoginVar := orgLogin
			repoNameVar := repoName
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
					Login: &orgLoginVar,
				},
				Repo: &github.Repository{
					Name: &repoNameVar,
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

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
			req.Header.Add(DeliveryIDHeader, "delivery-id")
			req.Header.Add(EventTypeHeader, tc.payloadType)
			req.Header.Add(ContentTypeHeader, tc.contentType)
			req.Header.Add(SHA256SignatureHeader, fmt.Sprintf("sha256=%s", createSignature([]byte(tc.payloadWebhookSecret), payload)))

			resp := httptest.NewRecorder()

			// Mock Redis client for registry operations
			db, mockRedis := redismock.NewClientMock()
			if tc.registryWorkerPools != nil {
				for key, pools := range tc.registryWorkerPools {
					poolsJSON, err := json.Marshal(pools)
					if err != nil {
						t.Fatalf("failed to marshal pools for key %s: %v", key, err)
					}
					mockRedis.ExpectGet(key).SetVal(string(poolsJSON))
				}
			}

			mockCloudBuildClient := &cloudbuild.MockClient{CreateBuildID: testGCBBuildID}
			mockGitHubClient := &gh.MockClient{
				GenerateRepoJITConfigF: func(ctx context.Context, installationID int64, org, repo, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
					return jit, nil
				},
				GenerateOrgJITConfigF: func(ctx context.Context, installationID int64, org, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
					return jit, nil
				},
			}

			cfg := &Config{
				GitHubAppID:                    "app-id",
				GitHubWebhookKeyMountPath:      "test-path",
				GitHubWebhookKeyName:           "test-key",
				KMSAppPrivateKeyID:             "test-kms-key",
				RunnerLocation:                 "us-central1", // Default runner location for tests
				RunnerProjectID:                "test-project",
				RunnerRepositoryID:             "test-repo",
				RunnerServiceAccount:           "test-sa",
				RunnerExecutionTimeoutSeconds:  tc.runnerExecutionTimeoutSeconds,
				RunnerIdleTimeoutSeconds:       tc.runnerIdleTimeoutSeconds,
				ExtraRunnerCount:               tc.extraSpawnNumber,
				RunnerImageTag:                 "latest",
				Environment:                    testEnv,
				GitHubAPIBaseURL:               "http://github-api-base-url",
				RunnerLabelAliases:             tc.runnerLabelAliases,
				SupportedRunnerLabels:          tc.supportedRunnerLabels,
				RunnerRegistryDefaultKeyPrefix: tc.runnerRegistryDefaultKeyPrefix,
				BackoffInitialDelay:            1 * time.Second,
				MaxRetryAttempts:               3,
				Runner404Enabled:               true,
				Runner404DefaultDisabled:       false,
				Runner404ImageName:             "runner-404",
				Runner404ImageTag:              "latest-404",
				Runner404Location:              "us-central1",
				Runner404ProjectID:             "404-project",
				Runner404ServiceAccount:        "404-sa",
			}

			// Configure WebhookClientOptions
			wco := &WebhookClientOptions{
				CloudBuildClientOverride: mockCloudBuildClient,
				GitHubClientOverride:     mockGitHubClient,
				OSFileReaderOverride: &MockFileReader{
					ReadFileFunc: func(filename string) ([]byte, error) {
						if filename == fmt.Sprintf("%s/%s", cfg.GitHubWebhookKeyMountPath, cfg.GitHubWebhookKeyName) {
							return []byte(serverGitHubWebhookSecret), nil
						}
						return nil, fmt.Errorf("unexpected file read: %s", filename)
					},
				},
				KeyManagementClientOverride: &MockKMSClient{},
			}

			srv, err := NewServer(ctx, nil, cfg, db, wco)
			if err != nil {
				t.Fatal(err)
			}

			// The test requires directly setting these, as NewServer sets up its own.
			srv.webhookSecret = []byte(tc.payloadWebhookSecret)
			srv.handleWebhook().ServeHTTP(resp, req)

			if got, want := resp.Code, tc.expStatusCode; got != want {
				t.Errorf("expected %d to be %d", got, want)
			}
			wantImageTag := "latest"
			if tc.exp404 {
				wantImageTag = "latest-404"
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
				if diff := cmp.Diff(tc.expGCBBuildIDs, r.GCBBuildIDs); diff != "" {
					t.Errorf("GCBBuildIDs mismatch (-want +got):\n%s", diff)
				}
			} else {
				// For all other cases (e.g., "in_progress", "completed", or errors),
				// we still expect a plain text response.
				if got, want := strings.TrimSpace(resp.Body.String()), tc.expRespBody; got != want {
					t.Errorf("expected %q to be %q", got, want)
				}
			}

			if tc.expectBuildCount == len(mockCloudBuildClient.CreateBuildReqs) {
				for _, buildReq := range mockCloudBuildClient.CreateBuildReqs {
					if got, want := buildReq.GetBuild().GetSubstitutions()["_IMAGE_TAG"], wantImageTag; got != want {
						t.Errorf("expected image tag %q to be %q", want, got)
					}
					if got, want := buildReq.GetBuild().GetTimeout().AsDuration(), expectedBuildTimeout; got != want {
						t.Errorf("expected build timeout %v to be %v", got, want)
					}
				}
			} else {
				t.Errorf("expected %d build(s) to be created, but %d build(s) were created with requests: %v",
					tc.expectBuildCount,
					len(mockCloudBuildClient.CreateBuildReqs),
					mockCloudBuildClient.CreateBuildReqs,
				)
			}
			if got, want := mockGitHubClient.GenerateRepoJITConfigCalls, tc.expectBuildCount; got != want {
				t.Errorf("expected %d calls to GenerateRepoJITConfig, but got %d", want, got)
			}
			if err := mockRedis.ExpectationsWereMet(); err != nil {
				t.Errorf("redis expectations not met: %v", err)
			}
		})
	}
}

// createSignature creates a HMAC 256 signature for the test request payload.
func createSignature(key, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
