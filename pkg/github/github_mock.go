// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package github

import (
	"context"

	"github.com/google/go-github/v69/github"
)

var _ Client = (*MockClient)(nil)

// MockClient is a mock of the GitHub client.
type MockClient struct {
	GenerateRepoJITConfigF     func(ctx context.Context, installationID int64, org, repo, runnerName, runnerLabel string) (*github.JITRunnerConfig, error)
	GenerateRepoJITConfigCalls int
	GenerateOrgJITConfigF      func(ctx context.Context, installationID int64, org, runnerName, runnerLabel string) (*github.JITRunnerConfig, error)
	GenerateOrgJITConfigCalls  int
}

// GenerateRepoJITConfig is a mock of the GenerateRepoJITConfig method.
func (m *MockClient) GenerateRepoJITConfig(ctx context.Context, installationID int64, org, repo, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	m.GenerateRepoJITConfigCalls++
	return m.GenerateRepoJITConfigF(ctx, installationID, org, repo, runnerName, runnerLabel)
}

// GenerateOrgJITConfig is a mock of the GenerateOrgJITConfig method.
func (m *MockClient) GenerateOrgJITConfig(ctx context.Context, installationID int64, org, runnerName, runnerLabel string) (*github.JITRunnerConfig, error) {
	m.GenerateOrgJITConfigCalls++
	return m.GenerateOrgJITConfigF(ctx, installationID, org, runnerName, runnerLabel)
}
