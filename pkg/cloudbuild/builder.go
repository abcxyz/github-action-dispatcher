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

package cloudbuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"

	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/pkg/logging"
)

// Client is a wrapper around the Google Cloud Build client.
type Client interface {
	ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error)
	CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest) (string, error)
	Close() error
}

// Builder defines the interface for creating a cloud build.
type Builder interface {
	CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest) (string, error)
}

// SDKBuilder is a builder that uses the Cloud Build SDK.
type SDKBuilder struct {
	client Client
}

// CreateBuild creates a build using the Cloud Build SDK.
func (b *SDKBuilder) CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest) (string, error) {
	id, err := b.client.CreateBuild(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create build via SDK: %w", err)
	}
	return id, nil
}

// httpBuild represents the JSON payload for a new build request.
type httpBuild struct {
	ID             string            `json:"id,omitempty"`
	RemoteConfig   string            `json:"remoteConfig,omitempty"`
	ServiceAccount string            `json:"serviceAccount,omitempty"`
	Timeout        string            `json:"timeout,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Substitutions  map[string]string `json:"substitutions,omitempty"`
	Options        *httpBuildOptions `json:"options,omitempty"`
}

// httpBuildOptions represents the build options in the JSON payload.
type httpBuildOptions struct {
	Logging            string          `json:"logging,omitempty"`
	LogStreamingOption string          `json:"logStreamingOption,omitempty"`
	Pool               *httpPoolOption `json:"pool,omitempty"`
}

// httpPoolOption represents the pool option in the JSON payload.
type httpPoolOption struct {
	Name string `json:"name,omitempty"`
}

// httpOperation represents the response from the create build API call.
type httpOperation struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata"`
}

// HTTPBuilder is a builder that uses raw HTTP requests.
type HTTPBuilder struct {
	httpClient   *http.Client
	remoteConfig string
	apiEndpoint  string
}

// CreateBuild creates a build using a raw HTTP request.
func (b *HTTPBuilder) CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest) (string, error) {
	logger := logging.FromContext(ctx)

	build := req.GetBuild()
	if build == nil {
		return "", fmt.Errorf("build is nil")
	}
	options := build.GetOptions()
	if options == nil {
		return "", fmt.Errorf("build options is nil")
	}

	var timeout string
	if build.GetTimeout() != nil {
		timeout = build.GetTimeout().AsDuration().String()
	}

	payload := &httpBuild{
		RemoteConfig:   b.remoteConfig,
		ServiceAccount: build.GetServiceAccount(),
		Timeout:        timeout,
		Tags:           build.GetTags(),
		Substitutions:  build.GetSubstitutions(),
		Options: &httpBuildOptions{
			Logging:            options.GetLogging().String(),
			LogStreamingOption: options.GetLogStreamingOption().String(),
			Pool: &httpPoolOption{
				Name: options.GetPool().GetName(),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal build request: %w", err)
	}

	url := fmt.Sprintf("%s/%s/builds", b.apiEndpoint, req.GetParent())

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	logger.DebugContext(ctx, "posting build request to gcb",
		"url", url,
		"body", string(body))

	resp, err := b.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send build request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var op httpOperation
	if err := json.NewDecoder(resp.Body).Decode(&op); err != nil {
		return "", fmt.Errorf("failed to decode build response: %w", err)
	}

	// The build ID is in the metadata.
	// The metadata is a map[string]any, and the build data is under the "build" key.
	buildData, ok := op.Metadata["build"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("failed to find build data in operation metadata")
	}

	buildID, ok := buildData["id"].(string)
	if !ok {
		return "", fmt.Errorf("failed to find build id in operation metadata")
	}

	return buildID, nil
}

// NewBuilder returns a new builder based on the worker pool configuration.
func NewBuilder(poolInfo *registry.WorkerPoolInfo, sdkClient Client, httpClient *http.Client) Builder {
	if poolInfo != nil && poolInfo.PoolType == "trusted" && poolInfo.RemoteConfig != "" {
		return &HTTPBuilder{
			httpClient:   httpClient,
			remoteConfig: poolInfo.RemoteConfig,
			apiEndpoint:  "https://cloudbuild.googleapis.com/v1",
		}
	}
	return &SDKBuilder{client: sdkClient}
}
