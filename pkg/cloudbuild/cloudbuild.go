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
	"context"
	"errors"
	"fmt"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/abcxyz/github-action-dispatcher/pkg/retry"
)

// Client is a wrapper around the Google Cloud Build client.
type Client interface {
	ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error)
	CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest, opts ...gax.CallOption) error
	Close() error
}

// cloudbuildClient is the implementation of the Client interface.
type cloudbuildClient struct {
	client    *cloudbuild.Client
	retryCfg *retry.BackoffConfig
}

// NewClient creates a new Cloud Build client.
func NewClient(ctx context.Context, cfg *retry.BackoffConfig, opts ...option.ClientOption) (Client, error) {
	client, err := cloudbuild.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new cloud build client: %w", err)
	}

	return &cloudbuildClient{
		client:    client,
		retryCfg: cfg,
	}, nil
}

// ListWorkerPools lists the worker pools for a given project.
func (c *cloudbuildClient) ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error) {
	var pools []*cloudbuildpb.WorkerPool

	it := c.client.ListWorkerPools(ctx, &cloudbuildpb.ListWorkerPoolsRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", projectID, location),
	}, retry.NewGRPCCallOption(c.retryCfg))
	for {
		pool, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list worker pools for project %s: %w", projectID, err)
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

// CreateBuild creates a new build.
func (c *cloudbuildClient) CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest, opts ...gax.CallOption) error {
	opts = append(opts, retry.NewGRPCCallOption(c.retryCfg))
	if _, err := c.client.CreateBuild(ctx, req, opts...); err != nil {
		return fmt.Errorf("failed to create cloud build build: %w", err)
	}
	return nil
}

// Close closes the client.
func (c *cloudbuildClient) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("failed to close CloudBuild client: %w", err)
	}
	return nil
}
