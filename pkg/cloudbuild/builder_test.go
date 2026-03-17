// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package cloudbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
)

func TestNewBuilder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		poolInfo     *registry.WorkerPoolInfo
		expectedType string
	}{
		{
			name:         "nil_pool_info",
			poolInfo:     nil,
			expectedType: "*cloudbuild.SDKBuilder",
		},
		{
			name: "private_pool",
			poolInfo: &registry.WorkerPoolInfo{
				PoolType: "private",
			},
			expectedType: "*cloudbuild.SDKBuilder",
		},
		{
			name: "trusted_pool_no_remote_config",
			poolInfo: &registry.WorkerPoolInfo{
				PoolType: "trusted",
			},
			expectedType: "*cloudbuild.SDKBuilder",
		},
		{
			name: "trusted_pool_with_remote_config",
			poolInfo: &registry.WorkerPoolInfo{
				PoolType:     "trusted",
				RemoteConfig: "test-config",
			},
			expectedType: "*cloudbuild.HTTPBuilder",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			builder := NewBuilder(tc.poolInfo, nil, nil)

			actualType := fmt.Sprintf("%T", builder)
			if actualType != tc.expectedType {
				t.Errorf("NewBuilder() returned builder of type %s, want %s", actualType, tc.expectedType)
			}
		})
	}
}

func TestHTTPBuilder_CreateBuild(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testBuildID := "test-build-id"
	testProjectID := "test-project-id"
	testLocation := "us-central1"
	testParent := fmt.Sprintf("projects/%s/locations/%s", testProjectID, testLocation)
	testRemoteConfig := "test-remote-config"

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request method and URL
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		expectedURL := fmt.Sprintf("/%s/builds", testParent)
		if r.URL.Path != expectedURL {
			t.Errorf("expected URL path %s, got %s", expectedURL, r.URL.Path)
		}

		// Decode the request body
		var build httpBuild
		if err := json.NewDecoder(r.Body).Decode(&build); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		// Check the payload
		if build.RemoteConfig != testRemoteConfig {
			t.Errorf("expected remote config %s, got %s", testRemoteConfig, build.RemoteConfig)
		}
		if build.Substitutions["_FOO"] != "bar" {
			t.Errorf("expected substitution _FOO to be bar, got %s", build.Substitutions["_FOO"])
		}

		// Send the response
		w.Header().Set("Content-Type", "application/json")
		op := &httpOperation{
			Name: "operations/test-operation",
			Metadata: map[string]any{
				"build": map[string]any{
					"id": testBuildID,
				},
			},
		}
		if err := json.NewEncoder(w).Encode(op); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	builder := &HTTPBuilder{
		httpClient:   server.Client(),
		remoteConfig: testRemoteConfig,
		apiEndpoint:  server.URL,
	}

	req := &cloudbuildpb.CreateBuildRequest{
		Parent:    testParent,
		ProjectId: testProjectID,
		Build: &cloudbuildpb.Build{
			Timeout: durationpb.New(10 * time.Minute),
			Substitutions: map[string]string{
				"_FOO": "bar",
			},
			Options: &cloudbuildpb.BuildOptions{},
		},
	}

	buildID, err := builder.CreateBuild(ctx, req)
	if err != nil {
		t.Fatalf("CreateBuild() returned error: %v", err)
	}

	if buildID != testBuildID {
		t.Errorf("CreateBuild() returned build ID %s, want %s", buildID, testBuildID)
	}
}

func TestHTTPBuilder_CreateBuild_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a mock server that always returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	builder := &HTTPBuilder{
		httpClient:   server.Client(),
		remoteConfig: "test-config",
		apiEndpoint:  server.URL,
	}

	req := &cloudbuildpb.CreateBuildRequest{
		Parent:    "projects/test-project/locations/us-central1",
		ProjectId: "test-project",
		Build: &cloudbuildpb.Build{
			Options: &cloudbuildpb.BuildOptions{},
		},
	}

	_, err := builder.CreateBuild(ctx, req)
	if err == nil {
		t.Fatal("CreateBuild() did not return an error")
	}
	if diff := cmp.Diff("unexpected status code: 500", err.Error()); diff != "" {
		t.Errorf("CreateBuild() returned wrong error message (-want +got):\n%s", diff)
	}
}
