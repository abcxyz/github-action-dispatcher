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

package discovery

import (
	"context"
	"errors"
	"fmt"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"google.golang.org/api/iterator"
)

type cloudBuildClient interface {
	ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error)
}

// cloudBuildClientImpl provides a client for interacting with the Cloud Build API.
type cloudBuildClientImpl struct {
	client *cloudbuild.Client
}

// newCloudBuildClient creates a new cloudBuildClient.
func newCloudBuildClient(ctx context.Context) (cloudBuildClient, error) {
	client, err := cloudbuild.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud build client: %w", err)
	}
	return &cloudBuildClientImpl{client: client}, nil
}

// ListWorkerPools lists the worker pools for a given project.
func (c *cloudBuildClientImpl) ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error) {
	var pools []*cloudbuildpb.WorkerPool
	it := c.client.ListWorkerPools(ctx, &cloudbuildpb.ListWorkerPoolsRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", projectID, location),
	})
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
