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
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	goretry "github.com/sethvargo/go-retry"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/abcxyz/pkg/logging"
)

// Client is a wrapper around the Google Cloud Build client.
type Client interface {
	ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error)
	CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest) error
	Close() error
}

// cloudbuildClient is the implementation of the Client interface.
type cloudbuildClient struct {
	client              *cloudbuild.Client
	backoffInitialDelay time.Duration
	maxRetryAttempts    int
}

// NewClient creates a new Cloud Build client.
func NewClient(ctx context.Context, backoffInitialDelay time.Duration, maxRetryAttempts int, opts ...option.ClientOption) (Client, error) {
	client, err := cloudbuild.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new cloud build client: %w", err)
	}

	return &cloudbuildClient{
		client:              client,
		backoffInitialDelay: backoffInitialDelay,
		maxRetryAttempts:    maxRetryAttempts,
	}, nil
}

// ListWorkerPools lists the worker pools for a given project.
func (c *cloudbuildClient) ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error) {
	logger := logging.FromContext(ctx)
	var pools []*cloudbuildpb.WorkerPool

	it := c.client.ListWorkerPools(ctx, &cloudbuildpb.ListWorkerPoolsRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", projectID, location),
	})

	backoff := c.newBackoff()

	for {
		var pool *cloudbuildpb.WorkerPool
		var nextErr error

		if err := goretry.Do(ctx, backoff, func(ctx context.Context) error {
			pool, nextErr = it.Next()
			if errors.Is(nextErr, iterator.Done) {
				return nil
			}
			if nextErr != nil {
				logger.WarnContext(ctx, "retrying due to ListWorkerPools failure", "error", nextErr)
				return goretry.RetryableError(fmt.Errorf("failed to get next worker pool: %w", nextErr))
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("failed to list worker pools after retries: %w", err)
		}

		// Break the outer for {} loop as iteration is complete.
		if errors.Is(nextErr, iterator.Done) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("unretryable error from cloud build iterator: %w", nextErr)
		}

		pools = append(pools, pool)
	}

	return pools, nil
}

// CreateBuild creates a new build.
func (c *cloudbuildClient) CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest) error {
	logger := logging.FromContext(ctx)
	backoff := c.newBackoff()

	if err := goretry.Do(ctx, backoff, func(ctx context.Context) error {
		if _, err := c.client.CreateBuild(ctx, req); err != nil {
			logger.WarnContext(ctx, "retrying due to CreateBuild failure", "error", err)
			return goretry.RetryableError(fmt.Errorf("failed to create Cloud Build build: %w", err))
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create Cloud Build build after retries: %w", err)
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

// newBackoff creates a new goretry.Backoff instance with the client's configured initial delay and max retries.
func (c *cloudbuildClient) newBackoff() goretry.Backoff {
	backoff := goretry.NewExponential(c.backoffInitialDelay)
	if c.maxRetryAttempts >= 0 {
		backoff = goretry.WithMaxRetries(uint64(c.maxRetryAttempts), backoff)
	}
	return backoff
}
