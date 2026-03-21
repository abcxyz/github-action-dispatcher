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
	"strings"
	"testing"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/go-redis/redismock/v8"

	"github.com/abcxyz/github-action-dispatcher/pkg/assetinventory"
	"github.com/abcxyz/github-action-dispatcher/pkg/cloudbuild"
	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

const (
	testProjectID1                     = "my-project"
	testProjectNumber1                 = "111111-my-project"
	testProjectID2                     = "another-project"
	testProjectNumber2                 = "222222-another-project"
	testProjectID3                     = "labeled-project-e2-small"
	testProjectNumber3                 = "333333-labeled-project-e2-small"
	testLocation                       = "us-central1"
	testJobRunsOnE2Medium              = "e2-medium"
	testJobRunsOnE2Small               = "e2-small"
	testJobRunsOnE2LargeStale          = "e2-large-stale"
	testWorkerPoolID1                  = "my-worker-pool-e2-medium"
	testWorkerPoolID2                  = "my-worker-pool-e2-medium-2"
	testWorkerPoolID3                  = "my-worker-pool-e2-small"
	testGCPFolderID                    = "12345"
	testRunnerRegistryDefaultKeyPrefix = "default"
	testWildcardOrg                    = "wildcard-org"
	testSemiWildcardOrg                = "semi-wildcard-org"
)

// newMockWorkerPool creates a new mock cloudbuildpb.WorkerPool for testing.
func newMockWorkerPool(projectIdentifier, location, poolID, machineType string) *cloudbuildpb.WorkerPool {
	return &cloudbuildpb.WorkerPool{
		Name: fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", projectIdentifier, location, poolID),
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
		cloudbuildMock     *cloudbuild.MockClient
		assetInventoryMock *assetinventory.MockClient
		expErr             string
		expRegistrySets    map[string][]registry.WorkerPoolInfo
		expRegistryDels    []string
		expectRedis        bool
	}{
		{
			name: "success_no_cache_read_new_registry_write",
			config: &Config{
				AllowedGithubOrgScopes:         "default",
				AllowedJobRunsOn:               strings.Join([]string{testJobRunsOnE2Medium, testJobRunsOnE2Small}, ","),
				AllowedPoolLocations:           "us-central1",
				AllowedPoolAvailabilities:      strings.Join([]string{poolAvailabilityAvailable, poolAvailabilityUnavailable}, ","),
				AllowedPoolTypes:               "trusted",
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &cloudbuild.MockClient{
				WorkerPools: []*cloudbuildpb.WorkerPool{
					newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium),
					newMockWorkerPool(testProjectNumber2, testLocation, testWorkerPoolID2, testJobRunsOnE2Medium),
					newMockWorkerPool(testProjectNumber3, testLocation, testWorkerPoolID3, testJobRunsOnE2Small),
				},
			},
			assetInventoryMock: &assetinventory.MockClient{
				StubProjects: []*assetinventory.ProjectInfo{
					{
						ProjectID: testProjectID1,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testRunnerRegistryDefaultKeyPrefix,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Medium,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
					{
						ProjectID: testProjectID2,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testRunnerRegistryDefaultKeyPrefix,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Medium,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
					{
						ProjectID: testProjectID3,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testRunnerRegistryDefaultKeyPrefix,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Small,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
				},
			},
			expRegistrySets: map[string][]registry.WorkerPoolInfo{
				testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testJobRunsOnE2Medium): func() []registry.WorkerPoolInfo {
					pools := []registry.WorkerPoolInfo{
						{
							Name:          newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium).GetName(),
							ProjectID:     testProjectID1,
							ProjectNumber: testProjectNumber1,
							Location:      testLocation,
							PoolType:      poolTypeTrusted,
						},
						{
							Name:          newMockWorkerPool(testProjectNumber2, testLocation, testWorkerPoolID2, testJobRunsOnE2Medium).GetName(),
							ProjectID:     testProjectID2,
							ProjectNumber: testProjectNumber2,
							Location:      testLocation,
							PoolType:      poolTypeTrusted,
						},
					}
					sort.Slice(pools, func(i, j int) bool {
						return pools[i].Name < pools[j].Name
					})
					return pools
				}(),
				testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testJobRunsOnE2Small): {
					{
						Name:          newMockWorkerPool(testProjectNumber3, testLocation, testWorkerPoolID3, testJobRunsOnE2Small).GetName(),
						ProjectID:     testProjectID3,
						ProjectNumber: testProjectNumber3,
						Location:      testLocation,
						PoolType:      poolTypeTrusted,
					},
				},
			},
			// This test simulates a scenario where a stale worker pool key exists
			// in the registry and should be deleted because it's no longer discovered.
			expRegistryDels: []string{testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testJobRunsOnE2LargeStale)},
			expectRedis:     true,
		},
		{
			name: "success_with_remote_config",
			config: &Config{
				AllowedGithubOrgScopes:         "default",
				AllowedJobRunsOn:               strings.Join([]string{testJobRunsOnE2Medium, testJobRunsOnE2Small}, ","),
				AllowedPoolLocations:           "us-central1",
				AllowedPoolAvailabilities:      strings.Join([]string{poolAvailabilityAvailable, poolAvailabilityUnavailable}, ","),
				AllowedPoolTypes:               "trusted,private",
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &cloudbuild.MockClient{
				WorkerPools: []*cloudbuildpb.WorkerPool{
					newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium),
					newMockWorkerPool(testProjectNumber3, testLocation, testWorkerPoolID3, testJobRunsOnE2Small),
				},
			},
			assetInventoryMock: &assetinventory.MockClient{
				StubProjects: []*assetinventory.ProjectInfo{
					{
						ProjectID: testProjectID1,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testRunnerRegistryDefaultKeyPrefix,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Medium,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
					{
						ProjectID: testProjectID3,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testRunnerRegistryDefaultKeyPrefix,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Small,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
				},
			},
			expRegistrySets: map[string][]registry.WorkerPoolInfo{
				testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testJobRunsOnE2Medium): {
					{
						Name:          newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium).GetName(),
						ProjectID:     testProjectID1,
						ProjectNumber: testProjectNumber1,
						Location:      testLocation,
						PoolType:      poolTypeTrusted,
					},
				},
				testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testJobRunsOnE2Small): {
					{
						Name:          newMockWorkerPool(testProjectNumber3, testLocation, testWorkerPoolID3, testJobRunsOnE2Small).GetName(),
						ProjectID:     testProjectID3,
						ProjectNumber: testProjectNumber3,
						Location:      testLocation,
						PoolType:      poolTypeTrusted,
					},
				},
			},
			expectRedis: true,
		},
		{
			name: "success_wildcard",
			config: &Config{
				AllowedGithubOrgScopes:         "*",
				AllowedJobRunsOn:               testJobRunsOnE2Medium,
				AllowedPoolLocations:           "us-central1",
				AllowedPoolAvailabilities:      strings.Join([]string{poolAvailabilityAvailable, poolAvailabilityUnavailable}, ","),
				AllowedPoolTypes:               "trusted",
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &cloudbuild.MockClient{
				WorkerPools: []*cloudbuildpb.WorkerPool{
					newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium),
				},
			},
			assetInventoryMock: &assetinventory.MockClient{
				StubProjects: []*assetinventory.ProjectInfo{
					{
						ProjectID: testProjectID1,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testWildcardOrg,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Medium,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
				},
			},
			expRegistrySets: map[string][]registry.WorkerPoolInfo{
				testRegistryKey(testWildcardOrg, testJobRunsOnE2Medium): {
					{
						Name:          newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium).GetName(),
						ProjectID:     testProjectID1,
						ProjectNumber: testProjectNumber1,
						Location:      testLocation,
						PoolType:      poolTypeTrusted,
					},
				},
			},
			expectRedis: true,
		},
		{
			name: "success_mixed_wildcard",
			config: &Config{
				AllowedGithubOrgScopes:         "default,semi-*-org,*",
				AllowedJobRunsOn:               strings.Join([]string{testJobRunsOnE2Medium, testJobRunsOnE2Small}, ","),
				AllowedPoolLocations:           "us-central1",
				AllowedPoolAvailabilities:      strings.Join([]string{poolAvailabilityAvailable, poolAvailabilityUnavailable}, ","),
				AllowedPoolTypes:               "trusted",
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &cloudbuild.MockClient{
				WorkerPools: []*cloudbuildpb.WorkerPool{
					newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium),
					newMockWorkerPool(testProjectNumber2, testLocation, testWorkerPoolID2, testJobRunsOnE2Medium),
					newMockWorkerPool(testProjectNumber3, testLocation, testWorkerPoolID3, testJobRunsOnE2Small),
				},
			},
			assetInventoryMock: &assetinventory.MockClient{
				StubProjects: []*assetinventory.ProjectInfo{
					{
						ProjectID: testProjectID1,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testRunnerRegistryDefaultKeyPrefix,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Medium,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
					{
						ProjectID: testProjectID2,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testSemiWildcardOrg,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Medium,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
					{
						ProjectID: testProjectID3,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testWildcardOrg,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Small,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
				},
			},
			expRegistrySets: map[string][]registry.WorkerPoolInfo{
				testRegistryKey(testRunnerRegistryDefaultKeyPrefix, testJobRunsOnE2Medium): {
					{
						Name:          newMockWorkerPool(testProjectNumber1, testLocation, testWorkerPoolID1, testJobRunsOnE2Medium).GetName(),
						ProjectID:     testProjectID1,
						ProjectNumber: testProjectNumber1,
						Location:      testLocation,
						PoolType:      poolTypeTrusted,
					},
				},
				testRegistryKey(testSemiWildcardOrg, testJobRunsOnE2Medium): {
					{
						Name:          newMockWorkerPool(testProjectNumber2, testLocation, testWorkerPoolID2, testJobRunsOnE2Medium).GetName(),
						ProjectID:     testProjectID2,
						ProjectNumber: testProjectNumber2,
						Location:      testLocation,
						PoolType:      poolTypeTrusted,
					},
				},
				testRegistryKey(testWildcardOrg, testJobRunsOnE2Small): {
					{
						Name:          newMockWorkerPool(testProjectNumber3, testLocation, testWorkerPoolID3, testJobRunsOnE2Small).GetName(),
						ProjectID:     testProjectID3,
						ProjectNumber: testProjectNumber3,
						Location:      testLocation,
						PoolType:      poolTypeTrusted,
					},
				},
			},
			expectRedis: true,
		},
		{
			name: "projects_error",
			config: &Config{
				AllowedGithubOrgScopes:         "default",
				AllowedJobRunsOn:               "self-hosted",
				AllowedPoolLocations:           "us-central1",
				AllowedPoolAvailabilities:      strings.Join([]string{poolAvailabilityAvailable, poolAvailabilityUnavailable}, ","),
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &cloudbuild.MockClient{},
			assetInventoryMock: &assetinventory.MockClient{
				ListProjectsErr: fmt.Errorf("failed to get projects"),
			},
			expErr:      `failed to list projects: failed to get projects`,
			expectRedis: false,
		},
		{
			name: "list_worker_pools_error",
			config: &Config{
				AllowedGithubOrgScopes:         "default",
				AllowedJobRunsOn:               testJobRunsOnE2Medium,
				AllowedPoolLocations:           "us-central1",
				AllowedPoolAvailabilities:      strings.Join([]string{poolAvailabilityAvailable, poolAvailabilityUnavailable}, ","),
				AllowedPoolTypes:               "trusted",
				GCPFolderID:                    testGCPFolderID,
				RunnerRegistryDefaultKeyPrefix: testRunnerRegistryDefaultKeyPrefix,
			},
			cloudbuildMock: &cloudbuild.MockClient{
				ListWorkerPoolsErr: fmt.Errorf("failed to list worker pools"),
			},
			assetInventoryMock: &assetinventory.MockClient{
				StubProjects: []*assetinventory.ProjectInfo{
					{
						ProjectID: testProjectID1,
						Labels: map[string]string{
							githubOrgScopeGCPProjectLabelKey:   testRunnerRegistryDefaultKeyPrefix,
							jobRunsOnGCPProjectLabelKey:        testJobRunsOnE2Medium,
							poolLocationGCPProjectLabelKey:     testLocation,
							poolAvailabilityGCPProjectLabelKey: poolAvailabilityAvailable,
							poolTypeGCPProjectLabelKey:         poolTypeTrusted,
						},
					},
				},
			},
			expRegistrySets: map[string][]registry.WorkerPoolInfo{},
			expErr:          `failed to build registry: failed to list worker pools for project my-project: failed to list worker pools`,
			expectRedis:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			db, mock := redismock.NewClientMock()
			labels := make(map[string][]string)
			labels[githubOrgScopeGCPProjectLabelKey] = tc.config.GetAllowedGithubOrgScopes()
			labels[jobRunsOnGCPProjectLabelKey] = tc.config.GetAllowedJobRunsOn()
			labels[poolLocationGCPProjectLabelKey] = tc.config.GetAllowedPoolLocations()
			labels[poolAvailabilityGCPProjectLabelKey] = tc.config.GetAllowedPoolAvailabilities()
			labels[poolTypeGCPProjectLabelKey] = tc.config.GetAllowedPoolTypes()

			rd := &RunnerDiscovery{
				cbc:                            tc.cloudbuildMock,
				aic:                            tc.assetInventoryMock,
				rc:                             db,
				config:                         tc.config,
				gcpRunnerAllowedProjectLabels:  labels,
				gcpRunnerIgnoredProjectLabels:  tc.config.GetIgnoredGCPProjectLabelsSet(),
				gcpRunnerOptionalProjectLabels: tc.config.GetOptionalGCPProjectLabelsSet(),
			}

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
