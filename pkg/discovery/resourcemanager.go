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

	"google.golang.org/api/cloudresourcemanager/v3"
)

type resourceManagerClient interface {
	Projects() projectsService
}

type projectsService interface {
	List() projectsListCall
	Search() projectsSearchCall
}

type projectsListCall interface {
	Parent(string) projectsListCall
	Pages(context.Context, func(*cloudresourcemanager.ListProjectsResponse) error) error
}

type projectsSearchCall interface {
	Query(string) projectsSearchCall
	Pages(context.Context, func(*cloudresourcemanager.SearchProjectsResponse) error) error
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

func (c *resourceManagerClientImpl) Projects() projectsService {
	return &projectsServiceImpl{service: c.service}
}

type projectsServiceImpl struct {
	service *cloudresourcemanager.Service
}

func (s *projectsServiceImpl) List() projectsListCall {
	return &projectsListCallImpl{call: s.service.Projects.List()}
}

type projectsListCallImpl struct {
	call *cloudresourcemanager.ProjectsListCall
}

func (c *projectsListCallImpl) Parent(parent string) projectsListCall {
	c.call.Parent(parent)
	return c
}

func (c *projectsListCallImpl) Pages(ctx context.Context, f func(*cloudresourcemanager.ListProjectsResponse) error) error {
	if err := c.call.Pages(ctx, f); err != nil {
		return fmt.Errorf("failed to get projects pages: %w", err)
	}
	return nil
}

func (s *projectsServiceImpl) Search() projectsSearchCall {
	return &projectsSearchCallImpl{call: s.service.Projects.Search()}
}

type projectsSearchCallImpl struct {
	call *cloudresourcemanager.ProjectsSearchCall
}

func (c *projectsSearchCallImpl) Query(query string) projectsSearchCall {
	c.call.Query(query)
	return c
}

func (c *projectsSearchCallImpl) Pages(ctx context.Context, f func(*cloudresourcemanager.SearchProjectsResponse) error) error {
	if err := c.call.Pages(ctx, f); err != nil {
		return fmt.Errorf("failed to search projects pages: %w", err)
	}
	return nil
}
