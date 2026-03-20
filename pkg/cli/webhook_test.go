// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2000
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sethvargo/go-envconfig"
	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"

	"github.com/abcxyz/github-action-dispatcher/pkg/webhook"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/serving"
	"github.com/abcxyz/pkg/testutil"
)

func TestWebhookServerCommand(t *testing.T) {
	t.Parallel()

	ctx := logging.WithLogger(t.Context(), logging.TestLogger(t))

	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		expErr   string
		fileMock *ReadFileResErr
	}{
		{
			name:   "too_many_args",
			args:   []string{"foo"},
			expErr: `unexpected arguments: ["foo"]`,
		},
		{
			name: "invalid_config_github_app_id",

			env: map[string]string{},

			expErr: `GITHUB_APP_ID is required`,
		},
		{
			name: "invalid_config_webhook_key_mount_path",
			env: map[string]string{
				"RUNNER_LOCATION": "runner-location",
				"GITHUB_APP_ID":   "github-app-id",
			},
			expErr: `WEBHOOK_KEY_MOUNT_PATH is required`,
		},
		{
			name: "invalid_config_webhook_key_name",
			env: map[string]string{
				"RUNNER_LOCATION":        "runner-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
			},
			expErr: `WEBHOOK_KEY_NAME is required`,
		},
		{
			name: "invalid_config_kms_app_private_key_id",
			env: map[string]string{
				"RUNNER_LOCATION":        "runner-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
			},
			expErr: `KMS_APP_PRIVATE_KEY_ID is required`,
		},
		{
			name: "invalid_config_runner_location",
			env: map[string]string{
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
				"KMS_APP_PRIVATE_KEY_ID": "kms-app-private-key-id",
			},
			expErr: `RUNNER_LOCATION is required`,
		},
		{
			name: "invalid_config_runner_project_id",
			env: map[string]string{
				"RUNNER_LOCATION":        "runner-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
				"KMS_APP_PRIVATE_KEY_ID": "kms-app-private-key-id",
			},
			expErr: `RUNNER_PROJECT_ID is required`,
		},
		{
			name: "invalid_runner_repository_id",
			env: map[string]string{
				"RUNNER_LOCATION":        "runner-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
				"KMS_APP_PRIVATE_KEY_ID": "kms-app-private-key-id",
				"RUNNER_PROJECT_ID":      "project-id",
			},
			expErr: `RUNNER_REPOSITORY_ID is required`,
		},
		{
			name: "invalid_config_runner_service_account",
			env: map[string]string{
				"RUNNER_LOCATION":        "runner-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
				"KMS_APP_PRIVATE_KEY_ID": "kms-app-private-key-id",
				"RUNNER_PROJECT_ID":      "runner-project-id",
				"RUNNER_REPOSITORY_ID":   "runner-repo-id",
			},
			expErr: `RUNNER_SERVICE_ACCOUNT is required`,
		},
		{
			name: "happy_path_no_aliases",
			env: map[string]string{
				"BUILD_TIMEOUT_SECONDS":              "3600",
				"GITHUB_APP_ID":                      "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH":             "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":                   "key-name",
				"KMS_APP_PRIVATE_KEY_ID":             "kms-app-private-key-id",
				"RUNNER_EXECUTION_TIMEOUT_SECONDS":   "3600",
				"RUNNER_IDLE_TIMEOUT_SECONDS":        "300",
				"RUNNER_LOCATION":                    "runner-location",
				"RUNNER_PROJECT_ID":                  "runner-project-id",
				"RUNNER_REPOSITORY_ID":               "runner-repo-id",
				"RUNNER_SERVICE_ACCOUNT":             "mock-runner-service-account@test-project.iam.gserviceaccount.com",
				"RUNNER_WORKER_POOL_ID":              "projects/my-project-number/locations/us-central1/workerPools/my-pool",
				"SUPPORTED_RUNNER_LABELS":            "sh-ubuntu-latest",
				"RUNNER_REGISTRY_DEFAULT_KEY_PREFIX": "default",
			}, fileMock: &ReadFileResErr{
				Res: []byte("secret-value"),
			},
		},
		{
			name: "happy_path",
			env: map[string]string{
				"BUILD_TIMEOUT_SECONDS":              "3600",
				"GITHUB_APP_ID":                      "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH":             "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":                   "key-name",
				"KMS_APP_PRIVATE_KEY_ID":             "kms-app-private-key-id",
				"RUNNER_EXECUTION_TIMEOUT_SECONDS":   "3600",
				"RUNNER_IDLE_TIMEOUT_SECONDS":        "300",
				"RUNNER_LOCATION":                    "runner-location",
				"RUNNER_PROJECT_ID":                  "runner-project-id",
				"RUNNER_REPOSITORY_ID":               "runner-repo-id",
				"RUNNER_SERVICE_ACCOUNT":             "runner-service-account",
				"RUNNER_LABEL_ALIASES":               "self-hosted=sh-ubuntu-latest",
				"RUNNER_WORKER_POOL_ID":              "projects/my-project-number/locations/us-central1/workerPools/my-pool",
				"SUPPORTED_RUNNER_LABELS":            "self-hosted,sh-ubuntu-latest",
				"RUNNER_REGISTRY_DEFAULT_KEY_PREFIX": "default",
			}, fileMock: &ReadFileResErr{
				Res: []byte("secret-value"),
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, done := context.WithCancel(ctx)
			defer done()

			var cmd WebhookServerCommand
			cmd.testFlagSetOpts = []cli.Option{cli.WithLookupEnv(envconfig.MultiLookuper(
				envconfig.MapLookuper(tc.env),
				envconfig.MapLookuper(map[string]string{
					// Make the test choose a random port.
					"PORT": "0",
				}),
			).Lookup)}

			_, _, _ = cmd.Pipe()

			// Check if a fileMock is provided for happy path. If so, create and assign webhookClientOptions.
			if tc.fileMock != nil {
				cmd.testWebhookClientOptions = &webhook.WebhookClientOptions{
					OSFileReaderOverride: tc.fileMock,
					KeyManagementClientOverride: &MockKMSClient{},
				}
			}

			var srv *serving.Server
			var mux http.Handler
			var runErr error

			// Call RunUnstarted to get the actual error based on the command's internal logic
			// In happy path cases, this will return the configured server and mux.
			// In error cases, it will return the error.
			srv, mux, runErr = cmd.RunUnstarted(ctx, tc.args)

			if diff := testutil.DiffErrString(runErr, tc.expErr); diff != "" {
				t.Fatal(diff)
			}

			// If RunUnstarted returned an error, we're done with this test case.
			if runErr != nil {
				return
			}

			// For happy path, proceed with health check using the returned srv and mux.
			serverCtx, serverDone := context.WithCancel(ctx)
			defer serverDone()
			go func() {
				if err := srv.StartHTTPHandler(serverCtx, mux); err != nil {
					t.Error(err)
				}
			}()

			client := &http.Client{
				Timeout: 5 * time.Second,
			}

			uri := "http://" + srv.Addr() + "/healthz"
			req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if got, want := resp.StatusCode, http.StatusOK; got != want {
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				t.Errorf("expected status code %d to be %d: %s", got, want, string(b))
			}
		})
	}
// ReadFileResErr is a struct to hold the response and error from a ReadFile call.
type ReadFileResErr struct {
	Res []byte
	Err error
}

func (m *ReadFileResErr) ReadFile(filename string) ([]byte, error) {
	return m.Res, m.Err
}

// MockKMSClient is a mock implementation of the KMS client.
type MockKMSClient struct {
	webhook.KeyManagementClient
}

func (m *MockKMSClient) CreateSigner(ctx context.Context, kmsAppPrivateKeyID string) (*gcpkms.Signer, error) {
	return nil, nil
}

func (m *MockKMSClient) Close() error {
	return nil
}
