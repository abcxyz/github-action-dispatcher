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
	"testing"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"

	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestRunnerDiscovery_Run(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		config             *Config
		cloudbuildMock     *mockCloudBuildClient
		assetInventoryMock *mockAssetInventoryClient
		expErr             string
	}{
		{
			name: "success",
			config: &Config{
				LabelQuery:        []string{"env=test"},
				GCPOrganizationID: "12345",
			},
			cloudbuildMock: &mockCloudBuildClient{
				workerPools: []*cloudbuildpb.WorkerPool{
					{Name: "pool1"},
				},
			},
			assetInventoryMock: &mockAssetInventoryClient{
				projects: []string{"labeled-project"},
			},
		},
		{
			name: "projects_error",
			config: &Config{
				LabelQuery:        []string{"env=test"},
				GCPOrganizationID: "12345",
			},
			cloudbuildMock: &mockCloudBuildClient{},
			assetInventoryMock: &mockAssetInventoryClient{
				projectsErr: fmt.Errorf("failed to get projects"),
			},
			expErr: `failed to get projects: failed to get projects`,
		},
		{
			name: "list_worker_pools_error",
			config: &Config{
				LabelQuery:        []string{"env=test"},
				GCPOrganizationID: "12345",
			},
			cloudbuildMock: &mockCloudBuildClient{
				listWorkerPoolsErr: fmt.Errorf("failed to list worker pools"),
			},
			assetInventoryMock: &mockAssetInventoryClient{
				projects: []string{"my-project"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			rd := &RunnerDiscovery{
				cbc:    tc.cloudbuildMock,
				aic:    tc.assetInventoryMock,
				config: tc.config,
			}

			err := rd.Run(ctx)

			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
