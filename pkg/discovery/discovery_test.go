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
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/go-redis/redismock/v8"

	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

const (
	testProjectID1              = "my-project"
	testProjectID2              = "another-project"
	testLocation                = "us-central1"
	testMachineTypeE2Medium     = "e2-medium"
	testMachineTypeE2Small      = "e2-small"
	testMachineTypeE2LargeStale = "e2-large-stale"
	testWorkerPoolID1           = "my-worker-pool-e2-medium"
	testWorkerPoolID2           = "my-worker-pool-e2-medium-2"
	testWorkerPoolID3           = "my-worker-pool-e2-small"
	testGCPFolderID             = "12345"
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

// defaultRegistryKey generates a registry key in the "default-<machine-type>" format.
func defaultRegistryKey(machineType string) string {
	return fmt.Sprintf("default-%s", machineType)
}

func TestRunnerDiscovery_Run(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		config             *Config
		cloudbuildMock     *mockCloudBuildClient
		assetInventoryMock *mockAssetInventoryClient
		expErr             string
		expRegistrySets    map[string][]string // Key: machineType, Value: list of worker pool names
		expRegistryDels    []string            // Keys expected to be deleted
	}{
		{
			name: "success_no_cache_read_new_registry_write",
			config: &Config{
				LabelQuery:  []string{"env=test"},
				GCPFolderID: testGCPFolderID,
			},
			cloudbuildMock: &mockCloudBuildClient{
				workerPools: []*cloudbuildpb.WorkerPool{
					newMockWorkerPool(testProjectID1, testLocation, testWorkerPoolID1, testMachineTypeE2Medium),
					newMockWorkerPool(testProjectID2, testLocation, testWorkerPoolID2, testMachineTypeE2Medium),
					newMockWorkerPool(testProjectID1, testLocation, testWorkerPoolID3, testMachineTypeE2Small),
				},
			},
			assetInventoryMock: &mockAssetInventoryClient{
				projects: []*ProjectInfo{
					{ID: "labeled-project", Number: "labeled-project", Labels: map[string]string{}},
				},
			},
			expRegistrySets: map[string][]string{
				testMachineTypeE2Medium: {
					newMockWorkerPool(testProjectID1, testLocation, testWorkerPoolID1, testMachineTypeE2Medium).GetName(),
					newMockWorkerPool(testProjectID2, testLocation, testWorkerPoolID2, testMachineTypeE2Medium).GetName(),
				},
				testMachineTypeE2Small: {
					newMockWorkerPool(testProjectID1, testLocation, testWorkerPoolID3, testMachineTypeE2Small).GetName(),
				},
			},
			// This test simulates a scenario where a stale worker pool key exists
			// in the registry and should be deleted because it's no longer discovered.
			expRegistryDels: []string{defaultRegistryKey(testMachineTypeE2LargeStale)},
		},
		{
			name: "projects_error",
			config: &Config{
				LabelQuery:  []string{"env=test"},
				GCPFolderID: testGCPFolderID,
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
				LabelQuery:  []string{"env=test"},
				GCPFolderID: testGCPFolderID,
			},
			cloudbuildMock: &mockCloudBuildClient{
				listWorkerPoolsErr: fmt.Errorf("failed to list worker pools"),
			},
			assetInventoryMock: &mockAssetInventoryClient{
				projects: []*ProjectInfo{
					{ID: "my-project", Number: "my-project", Labels: map[string]string{}},
				},
			},
			expRegistrySets: map[string][]string{},
			expErr:          "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			db, mock := redismock.NewClientMock()

			if tc.expRegistrySets != nil {
				// Expect Scan for stale keys.
				if len(tc.expRegistryDels) > 0 {
					mock.ExpectScan(0, "default-*", 0).SetVal(tc.expRegistryDels, 0)
				} else {
					mock.ExpectScan(0, "default-*", 0).SetVal([]string{}, 0)
				}

				// Only expect a transaction if there are keys to delete or set.
				if len(tc.expRegistryDels) > 0 || len(tc.expRegistrySets) > 0 {
					// Begin pipeline for transactional update
					mock.ExpectTxPipeline()

					if len(tc.expRegistryDels) > 0 {
						// Expect DEL for stale keys
						mock.ExpectDel(tc.expRegistryDels...).SetVal(int64(len(tc.expRegistryDels)))
					}

					// Collect and sort keys for deterministic expectation setting
					var sortedMachineTypes []string
					for machineType := range tc.expRegistrySets {
						sortedMachineTypes = append(sortedMachineTypes, machineType)
					}
					sort.Strings(sortedMachineTypes)

					for _, machineType := range sortedMachineTypes {
						pools := tc.expRegistrySets[machineType]
						registryKey := defaultRegistryKey(machineType)
						poolsJSON, err := json.Marshal(pools)
						if err != nil {
							t.Fatalf("failed to marshal pools for machine type %s: %v", machineType, err)
						}
						mock.ExpectSet(registryKey, poolsJSON, 0).SetVal("OK")
					}

					// Expect the pipeline to be executed.
					// Since discovery.go now conditionally initiates the pipeline,
					// this ExpectTxPipelineExec will only be reached if there are actual commands.
					mock.ExpectTxPipelineExec()
				}
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
