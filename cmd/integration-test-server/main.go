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

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
	"github.com/abcxyz/pkg/serving"
	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"

	"github.com/google/github_actions_on_gcp/pkg/webhook"
)

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
	cfg := &webhook.Config{
		GitHubAppID:               "12345",
		KMSAppPrivateKeyID:        "dummy",
		RunnerLocation:            "us-central1",
		RunnerProjectID:           "test-project",
		RunnerRepositoryID:        "test-repo",
		RunnerServiceAccount:      "test-sa",
		GitHubWebhookKeyMountPath: "/tmp",
		GitHubWebhookKeyName:      "webhook-key",
	}

	h, err := renderer.New(ctx, nil,
		renderer.WithOnError(func(err error) {
			logging.FromContext(ctx).ErrorContext(ctx, "failed to render", "error", err)
		}))
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	opts := &webhook.WebhookClientOptions{
		OSFileReaderOverride: &webhook.MockFileReader{
			ReadFileMock: &webhook.ReadFileResErr{
				Res: []byte("test-github-webhook-secret"),
			},
		},
		CloudBuildClientOverride: &webhook.MockCloudBuildClient{},
		KeyManagementClientOverride: &webhook.MockKMSClient{
			CreateSignerMock: &webhook.CreateSignerRes{Res: &gcpkms.Signer{}},
		},
	}

	webhookServer, err := webhook.NewServer(ctx, h, cfg, opts)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	mux := webhookServer.Routes(ctx)

	server, err := serving.New("8080")
	if err != nil {
		return fmt.Errorf("failed to create serving infrastructure: %w", err)
	}

	return server.StartHTTPHandler(ctx, mux)
}
