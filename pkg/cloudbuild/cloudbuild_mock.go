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

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
)

var _ Client = (*MockClient)(nil)

// MockClient is a mock of the Client interface.
type MockClient struct {
	ListWorkerPoolsErr error
	CreateBuildErr     error
	WorkerPools        []*cloudbuildpb.WorkerPool
	CreateBuildReqs    []*cloudbuildpb.CreateBuildRequest
}

// ListWorkerPools is a mock of the ListWorkerPools method.
func (m *MockClient) ListWorkerPools(ctx context.Context, projectID, location string) ([]*cloudbuildpb.WorkerPool, error) {
	if m.ListWorkerPoolsErr != nil {
		return nil, m.ListWorkerPoolsErr
	}
	return m.WorkerPools, nil
}

// CreateBuild is a mock of the CreateBuild method.
func (m *MockClient) CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest) error {
	m.CreateBuildReqs = append(m.CreateBuildReqs, req)
	if m.CreateBuildErr != nil {
		return m.CreateBuildErr
	}
	return nil
}

// Close is a mock of the Close method.
func (m *MockClient) Close() error {
	return nil
}
