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

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abcxyz/github-action-dispatcher/pkg/webhook"
	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/renderer"
)

type mockFileReader struct {
	content []byte
	err     error
}

func (r *mockFileReader) ReadFile(filename string) ([]byte, error) {
	return r.content, r.err
}

const (
	testSecret = "test-secret"
)

func TestWebhookHarness(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// Create a mock KMS client.
	mockKMS := &webhook.MockKMSClient{}

	// Create a mock Cloud Build client.
	mockCloudBuild := &webhook.MockCloudBuildClient{}

	var fakeGitHub *httptest.Server
	fakeGitHub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("fakeGitHub received request: %s %s", r.Method, r.URL.Path)
		switch r.URL.Path {
		case "/app/installations/54321":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"access_tokens_url": "` + fakeGitHub.URL + `/app/installations/54321/access_tokens"}`)); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
		case "/app/installations/54321/access_tokens":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"token": "test-token"}`)); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
		case "/repos/test-org/test-repo/actions/runners/generate-jitconfig":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"encoded_jit_config": "test-jit-config"}`)); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeGitHub.Close()

	// Generate a new RSA private key for the test.
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	app, err := githubauth.NewApp("test-app", privateKey, githubauth.WithBaseURL(fakeGitHub.URL))
	if err != nil {
		t.Fatalf("failed to create github app: %v", err)
	}

	h, err := renderer.New(ctx, nil, renderer.WithDebug(true))
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}

	// Create a new server with the mock clients.
	opts := &webhook.WebhookClientOptions{
		CloudBuildClientOverride:    mockCloudBuild,
		KeyManagementClientOverride: mockKMS,
		OSFileReaderOverride: &mockFileReader{
			content: []byte(testSecret),
		},
	}

	cfg := &webhook.Config{
		Environment:      "autopush",
		RunnerImageTag:   "latest",
		GitHubAPIBaseURL: fakeGitHub.URL,
	}

	server, err := webhook.NewServer(ctx, h, cfg, opts)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	server.SetAppClient(app)

	// A valid payload for a "queued" event.
	validPayload := `{
		"action": "queued",
		"workflow_job": {
			"id": 123456789,
			"run_id": 987654321,
			"name": "test-job",
			"labels": ["self-hosted"],
			"created_at": "2025-07-12T00:00:00Z",
			"started_at": "2025-07-12T00:00:00Z"
		},
		"repository": { "name": "test-repo" },
		"organization": { "login": "test-org" },
		"installation": { "id": 54321 }
	}`

	// Generate a signature for the payload.
	mac := hmac.New(sha256.New, []byte(testSecret))
	if _, err := mac.Write([]byte(validPayload)); err != nil {
		t.Fatalf("failed to write payload to hmac: %v", err)
	}
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Create a response recorder.
	recorder := httptest.NewRecorder()

	// Create a new request.
	req, err := http.NewRequestWithContext(ctx, "POST", "/webhook", bytes.NewBufferString(validPayload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Set the required headers.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", signature)

	// Serve the request.
	server.Routes(ctx).ServeHTTP(recorder, req)

	// Check the response status code.
	if recorder.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d, body: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	// Check that the mock Cloud Build client was called.
	if mockCloudBuild.CreateBuildReq == nil {
		t.Error("expected CreateBuild to be called, but it was not")
	}
}
