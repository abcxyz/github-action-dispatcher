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

	"github.com/abcxyz/pkg/logging"
)

// RunnerDiscovery is the main struct for the runner-discovery job.
type RunnerDiscovery struct {
	cbc    cloudBuildClient
	crmc   resourceManagerClient
	config *Config
}

// NewRunnerDiscovery creates a new RunnerDiscovery instance.
func NewRunnerDiscovery(ctx context.Context, config *Config) (*RunnerDiscovery, error) {
	cbc, err := newCloudBuildClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud build client: %w", err)
	}

	crmc, err := newResourceManagerClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud resource manager client: %w", err)
	}

	return &RunnerDiscovery{
		cbc:    cbc,
		crmc:   crmc,
		config: config,
	}, nil
}

// Run is the main entrypoint for the runner-discovery job.
func (rd *RunnerDiscovery) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	projects, err := rd.getProjects(ctx)
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}

	for _, project := range projects {
		logger.InfoContext(ctx,
			"checking project for worker pools",
			"project", project)
		wps, err := rd.cbc.ListWorkerPools(ctx, project, "-")
		if err != nil {
			logger.ErrorContext(ctx,
				"failed to list worker pools",
				"project", project,
				"error", err)
			continue
		}

		for _, wp := range wps {
			logger.InfoContext(ctx,
				"found worker pool",
				"project", project,
				"worker_pool", wp.GetName(),
				"state", wp.GetState(),
				"config", wp.GetConfig())
		}
	}

	return nil
}

func (rd *RunnerDiscovery) getProjects(ctx context.Context) ([]string, error) {
	projects := make(map[string]struct{})

	queryParts := make([]string, 0, len(rd.config.LabelQuery))
	for _, label := range rd.config.LabelQuery {
		queryParts = append(queryParts, fmt.Sprintf("labels.%s", label))
	}
	labelQuery := strings.Join(queryParts, " ")
	query := fmt.Sprintf("parent:organizations/%s %s", rd.config.GCPOrganizationID, labelQuery)

	if err := rd.crmc.Projects().Search().Query(query).Pages(ctx, func(page *cloudresourcemanager.SearchProjectsResponse) error {
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
