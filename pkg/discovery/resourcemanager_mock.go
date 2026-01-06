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
	projects    []*cloudresourcemanager.Project
	projectsErr error
}

func (m *mockResourceManagerClient) Projects(ctx context.Context, gcpOrganizationID string, labelQuery []string) ([]string, error) {
	if m.projectsErr != nil {
		return nil, m.projectsErr
	}
	ids := make([]string, 0, len(m.projects))
	for _, p := range m.projects {
		ids = append(ids, p.ProjectId)
	}
	return ids, nil
}
