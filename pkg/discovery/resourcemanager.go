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
	"fmt"
	"strings"

	"google.golang.org/api/cloudresourcemanager/v3"
)

type resourceManagerClient interface {
	Projects(context.Context, string, []string) ([]string, error)
}

type resourceManagerClientImpl struct {
	service *cloudresourcemanager.Service
}

func newResourceManagerClient(ctx context.Context) (resourceManagerClient, error) {
	service, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create new resource manager service: %w", err)
	}
	return &resourceManagerClientImpl{service: service}, nil
}

func (c *resourceManagerClientImpl) Projects(ctx context.Context, gcpOrganizationID string, labelQuery []string) ([]string, error) {
	projects := make(map[string]struct{})

	queryParts := make([]string, 0, len(labelQuery))
	for _, label := range labelQuery {
		queryParts = append(queryParts, fmt.Sprintf("labels.%s", label))
	}
	labels := strings.Join(queryParts, " ")
	query := fmt.Sprintf("parent:organizations/%s %s", gcpOrganizationID, labels)

	if err := c.service.Projects.Search().Query(query).Pages(ctx, func(page *cloudresourcemanager.SearchProjectsResponse) error {
		for _, project := range page.Projects {
			projects[project.ProjectId] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to search projects with query %q: %w", query, err)
	}

	projectList := make([]string, 0, len(projects))
	for project := range projects {
		projectList = append(projectList, project)
	}

	return projectList, nil
}
