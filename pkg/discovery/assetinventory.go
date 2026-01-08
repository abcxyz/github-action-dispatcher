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
	"strings"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"google.golang.org/api/iterator"
)

type assetInventoryClient interface {
	Projects(context.Context, string, []string) ([]string, error)
}

type assetInventoryClientImpl struct {
	client *asset.Client
}

func newAssetInventoryClient(ctx context.Context) (assetInventoryClient, error) {
	client, err := asset.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create new asset inventory client: %w", err)
	}
	return &assetInventoryClientImpl{client: client}, nil
}

func (c *assetInventoryClientImpl) Projects(ctx context.Context, gcpOrganizationID string, labelQuery []string) ([]string, error) {
	query := fmt.Sprintf("ancestors:organizations/%s AND resource_type:cloudresourcemanager.googleapis.com/Project", gcpOrganizationID)
	if len(labelQuery) > 0 {
		query = fmt.Sprintf("%s AND %s", query, strings.Join(labelQuery, " AND "))
	}

	req := &assetpb.SearchAllResourcesRequest{
		Scope: fmt.Sprintf("organizations/%s", gcpOrganizationID),
		Query: query,
	}

	it := c.client.SearchAllResources(ctx, req)
	projects := make([]string, 0)
	for {
		resource, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get next resource: %w", err)
		}
		// The project ID is in the format "projects/12345", we want to extract the number.
		// It can also be in the format "//cloudresourcemanager.googleapis.com/projects/project-id-string"
		parts := strings.Split(resource.GetProject(), "/")
		projectID := parts[len(parts)-1]
		projects = append(projects, projectID)
	}

	return projects, nil
}
