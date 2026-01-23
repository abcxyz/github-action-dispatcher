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
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb" //nolint:gci
	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/go-redis/redismock/v8" //nolint:gci
)

const (
	testProjectID1                     = "my-project"
	testProjectID2                     = "another-project"
	testProjectID3                     = "labeled-project-e2-small"
	testLocation                       = "us-central1"
	testMachineTypeE2Medium            = "e2-medium"
	testMachineTypeE2Small             = "e2-small"
	testMachineTypeE2LargeStale        = "e2-large-stale"
	testWorkerPoolID1                  = "my-worker-pool-e2-medium"
	testWorkerPoolID2                  = "my-worker-pool-e2-medium-2"
	testWorkerPoolID3                  = "my-worker-pool-e2-small"
	testGCPFolderID                    = "12345"
	testRunnerRegistryDefaultKeyPrefix = "default"
)

// newMockWorkerPool creates a new mock cloudbuildpb.WorkerPool for testing.
func newMockWorkerPool(projectID, location, poolID, machineType string) *cloudbuildpb.WorkerPool {
	return &cloudbuildpb.WorkerPool{
		Name: fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", projectID, location, poolID),
		Config: &cloudbuildpb.WorkerPool_PrivatePoolV1Config{
			PrivatePoolV1Config: &cloudbuildpb.PrivatePoolV1Config{
				WorkerConfig: &cloudbuildpb.PrivatePoolV1Config_WorkerConfig{
					MachineType: machineType,
				},
			},
		},
	}
}

func testRegistryKey(prefix, machineType string) string {
	return fmt.Sprintf("%s:%s", prefix, machineType)
}

func TestRunnerDiscovery_Run(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		config             *Config
		cloudbuildMock     *mockCloudBuildClient
		assetInventoryMock *mockAssetInventoryClient
		expErr             string
		expRegistrySets    map[string][]registry.WorkerPoolInfo
		expRegistryDels    []string
		expectRedis        bool
	}{
		{
			name: "success_no_cache_read_new_registry_write",
			config: &Config{
				LabelQuery:                     []string{"env=test"},
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &mockCloudBuildClient{
				workerPools: []*cloudbuildpb.WorkerPool{
					newMockWorkerPool(testProjectID1, testLocation, testWorkerPoolID1, testMachineTypeE2Medium),
					newMockWorkerPool(testProjectID2, testLocation, testWorkerPoolID2, testMachineTypeE2Medium),
					newMockWorkerPool(testProjectID3, testLocation, testWorkerPoolID3, testMachineTypeE2Small),
				},
			},
			assetInventoryMock: &mockAssetInventoryClient{
				projects: []*ProjectInfo{
					{
						ID: testProjectID1,
						Labels: map[string]string{
							runnerTypeGCPProjectLabelKey:     testRunnerRegistryDefaultKeyPrefix,
							runnerLabelGCPProjectLabelKey:    testMachineTypeE2Medium,
							runnerLocationGCPProjectLabelKey: testLocation,
						},
					},
					{
						ID: testProjectID2,
						Labels: map[string]string{
							runnerTypeGCPProjectLabelKey:     testRunnerRegistryDefaultKeyPrefix,
							runnerLabelGCPProjectLabelKey:    testMachineTypeE2Medium,
							runnerLocationGCPProjectLabelKey: testLocation,
						},
					},
					{
						ID: testProjectID3,
						Labels: map[string]string{
							runnerTypeGCPProjectLabelKey:     testRunnerRegistryDefaultKeyPrefix,
							runnerLabelGCPProjectLabelKey:    testMachineTypeE2Small,
							runnerLocationGCPProjectLabelKey: testLocation,
						},
					},
				},
			},
			expRegistrySets: map[string][]registry.WorkerPoolInfo{
				testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testMachineTypeE2Medium): func() []registry.WorkerPoolInfo {
					pools := []registry.WorkerPoolInfo{
						{
							Name:      newMockWorkerPool(testProjectID1, testLocation, testWorkerPoolID1, testMachineTypeE2Medium).GetName(),
							ProjectID: testProjectID1,
						},
						{
							Name:      newMockWorkerPool(testProjectID2, testLocation, testWorkerPoolID2, testMachineTypeE2Medium).GetName(),
							ProjectID: testProjectID2,
						},
					}
					sort.Slice(pools, func(i, j int) bool {
						return pools[i].Name < pools[j].Name
					})
					return pools
				}(),
				testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testMachineTypeE2Small): {
					{
						Name:      newMockWorkerPool(testProjectID3, testLocation, testWorkerPoolID3, testMachineTypeE2Small).GetName(),
						ProjectID: testProjectID3,
					},
				},
			},
			// This test simulates a scenario where a stale worker pool key exists
			// in the registry and should be deleted because it's no longer discovered.
			expRegistryDels: []string{testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testMachineTypeE2LargeStale)},
			expectRedis:     true,
		},
		{
			name: "projects_error",
			config: &Config{
				LabelQuery:                     []string{"env=test"},
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &mockCloudBuildClient{},
			assetInventoryMock: &mockAssetInventoryClient{
				projectsErr: fmt.Errorf("failed to get projects"),
			},
			expErr:      `failed to get projects: failed to get projects`,
			expectRedis: false,
		},
		{
			name: "list_worker_pools_error",
			config: &Config{
				LabelQuery:                     []string{"env=test"},
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &mockCloudBuildClient{
				listWorkerPoolsErr: fmt.Errorf("failed to list worker pools"),
			},
			assetInventoryMock: &mockAssetInventoryClient{
				projects: []*ProjectInfo{
					{
						ID: testProjectID1,
						Labels: map[string]string{
							runnerTypeGCPProjectLabelKey:     testRunnerRegistryDefaultKeyPrefix,
							runnerLabelGCPProjectLabelKey:    testMachineTypeE2Medium,
							runnerLocationGCPProjectLabelKey: testLocation,
						},
					},
				},
			},
			expRegistrySets: map[string][]registry.WorkerPoolInfo{},
			expErr:          `failed to list worker pools`,
			expectRedis:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(t.Context(), logging.TestLogger(t))
			db, mock := redismock.NewClientMock()

			if tc.expectRedis {
				mock.ExpectScan(0, tc.config.RunnerRegistryDefaultKeyPrefix+":*", 0).SetVal(tc.expRegistryDels, 0)
				mock.ExpectTxPipeline()
				if len(tc.expRegistryDels) > 0 {
					mock.ExpectDel(tc.expRegistryDels...).SetVal(int64(len(tc.expRegistryDels)))
				}

				var keys []string
				for key := range tc.expRegistrySets {
					keys = append(keys, key)
				}
				sort.Strings(keys)

				for _, registryKey := range keys { // Iterate over sorted keys
					pools := tc.expRegistrySets[registryKey]
					poolsJSON, err := json.Marshal(pools)
					if err != nil {
						t.Fatalf("failed to marshal pools for key %s: %v", registryKey, err)
					}
					mock.ExpectSet(registryKey, poolsJSON, 0).SetVal("OK")
				}
				mock.ExpectTxPipelineExec()
			}
			rd := &RunnerDiscovery{
				cbc:    tc.cloudbuildMock,
				aic:    tc.assetInventoryMock,
				rc:     db,
				config: tc.config,
			}

			err := rd.Run(ctx)

			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("registry expectations not met: %v", err)
			}
		})
	}
}
