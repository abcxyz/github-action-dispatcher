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

package assetinventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	goretry "github.com/sethvargo/go-retry"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/abcxyz/pkg/logging"
)

const assetTypeProject = "cloudresourcemanager.googleapis.com/Project"

// ProjectInfo contains information about a GCP project.
type ProjectInfo struct {
	ProjectID string
	Labels    map[string]string
}

// Client is an interface for mocking asset inventory client.
type Client interface {
	FindProjects(ctx context.Context, gcpFolderID string, labelQuery []string) ([]*ProjectInfo, error)
	Close() error
}

// assetinventoryClient implements Client.
type assetinventoryClient struct {
	client              *asset.Client
	backoffInitialDelay time.Duration
	maxRetryAttempts    int
}

// NewClient creates a new asset inventory client.
func NewClient(ctx context.Context, backoffInitialDelay time.Duration, maxRetryAttempts int, opts ...option.ClientOption) (Client, error) {
	client, err := asset.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new asset inventory client: %w", err)
	}

	return &assetinventoryClient{
		client:              client,
		backoffInitialDelay: backoffInitialDelay,
		maxRetryAttempts:    maxRetryAttempts,
	}, nil
}

// Projects searches the Cloud Asset Inventory for projects within the given GCP folder
// ID that match the provided label query. It returns a slice of ProjectInfo
// containing the project ID, project number, and project labels.
func (c *assetinventoryClient) FindProjects(ctx context.Context, gcpFolderID string, labelQuery []string) ([]*ProjectInfo, error) {
	logger := logging.FromContext(ctx)
	var query strings.Builder
	for i, l := range labelQuery {
		if i > 0 {
			query.WriteString(" AND ")
		}
		query.WriteString("labels.")
		query.WriteString(l)
	}

	req := &assetpb.SearchAllResourcesRequest{
		Scope:      fmt.Sprintf("folders/%s", gcpFolderID),
		Query:      query.String(),
		AssetTypes: []string{assetTypeProject},
	}

	it := c.client.SearchAllResources(ctx, req)
	var projects []*ProjectInfo
	for {
		var resource *assetpb.ResourceSearchResult
		backoff := c.newBackoff()

		var nextErr error
		if err := goretry.Do(ctx, backoff, func(ctx context.Context) error {
			resource, nextErr = it.Next()
			if errors.Is(nextErr, iterator.Done) {
				return nil
			}
			if nextErr != nil {
				logger.WarnContext(ctx, "retrying due to FindProjects failure", "error", nextErr)
				return goretry.RetryableError(fmt.Errorf("failed to get next resource: %w", nextErr))
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("failed to get next resource after retries: %w", err)
		}

		if errors.Is(nextErr, iterator.Done) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("unretryable error from asset inventory iterator: %w", nextErr)
		}

		// e.g., //cloudresourcemanager.googleapis.com/projects/project-id
		nameParts := strings.Split(resource.GetName(), "/")
		projectID := nameParts[len(nameParts)-1]

		projects = append(projects, &ProjectInfo{
			ProjectID: projectID,
			Labels:    resource.GetLabels(),
		})
	}

	return projects, nil
}

// Close closes the asset inventory client.
func (c *assetinventoryClient) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("failed to close asset inventory client: %w", err)
	}
	return nil
}

// newBackoff creates a new goretry.Backoff instance with the client's configured initial delay and max attempts.
func (c *assetinventoryClient) newBackoff() goretry.Backoff {
	backoff := goretry.NewExponential(c.backoffInitialDelay)
	if c.maxRetryAttempts >= 0 {
		backoff = goretry.WithMaxRetries(uint64(c.maxRetryAttempts), backoff)
	}
	return backoff
}
