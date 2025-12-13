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

	"google.golang.org/api/cloudresourcemanager/v3"
)

type mockResourceManagerClient struct {
	projects          []*cloudresourcemanager.Project
	listProjectsErr   error
	searchProjectsErr error
}

func (m *mockResourceManagerClient) Projects() projectsService {
	return &mockProjectsService{m: m}
}

type mockProjectsService struct {
	m *mockResourceManagerClient
}

func (s *mockProjectsService) List() projectsListCall {
	return &mockProjectsListCall{s: s}
}

func (s *mockProjectsService) Search() projectsSearchCall {
	return &mockProjectsSearchCall{s: s}
}

type mockProjectsListCall struct {
	s *mockProjectsService
}

func (c *mockProjectsListCall) Parent(parent string) projectsListCall {
	return c
}

func (c *mockProjectsListCall) Pages(ctx context.Context, f func(*cloudresourcemanager.ListProjectsResponse) error) error {
	if c.s.m.listProjectsErr != nil {
		return c.s.m.listProjectsErr
	}

	return f(&cloudresourcemanager.ListProjectsResponse{
		Projects: c.s.m.projects,
	})
}

type mockProjectsSearchCall struct {
	s *mockProjectsService
}

func (c *mockProjectsSearchCall) Query(query string) projectsSearchCall {
	return c
}

func (c *mockProjectsSearchCall) Pages(ctx context.Context, f func(*cloudresourcemanager.SearchProjectsResponse) error) error {
	if c.s.m.searchProjectsErr != nil {
		return c.s.m.searchProjectsErr
	}

	return f(&cloudresourcemanager.SearchProjectsResponse{
		Projects: c.s.m.projects,
	})
}
