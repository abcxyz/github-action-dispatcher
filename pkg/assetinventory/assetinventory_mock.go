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
)

var _ Client = (*MockClient)(nil)

// MockClient is a mock of the Client interface.
type MockClient struct {
	ListProjectsErr error
	StubProjects    []*ProjectInfo
}

// Projects is a mock of the Projects method.
func (m *MockClient) FindProjects(ctx context.Context, folderID string, labelQuery []string) ([]*ProjectInfo, error) {
	if m.ListProjectsErr != nil {
		return nil, m.ListProjectsErr
	}
	return m.StubProjects, nil
}

// Close is a mock of the Close method.
func (m *MockClient) Close() error {
	return nil
}
