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
	"path/filepath"
	"sort"
	"strings"

	redisapi "github.com/go-redis/redis/v8"

	"github.com/abcxyz/github-action-dispatcher/pkg/assetinventory"
	"github.com/abcxyz/github-action-dispatcher/pkg/cloudbuild"
	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/pkg/logging"
)

// RunnerDiscovery is the main struct for the runner-discovery job.
type RunnerDiscovery struct {
	cbc                            cloudbuild.Client
	aic                            assetinventory.Client
	rc                             *redisapi.Client
	config                         *Config
	gcpRunnerAllowedProjectLabels  map[string][]string
	gcpRunnerIgnoredProjectLabels  map[string]struct{}
	gcpRunnerOptionalProjectLabels map[string]struct{}
}

func NewRunnerDiscovery(ctx context.Context, config *Config, rc *redisapi.Client) (*RunnerDiscovery, error) {
	aic, err := assetinventory.NewClient(ctx, config.BackoffInitialDelay, config.MaxRetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud asset inventory client: %w", err)
	}

	cbc, err := cloudbuild.NewClient(ctx, config.BackoffInitialDelay, config.MaxRetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud build client: %w", err)
	}

	labels := make(map[string][]string)
	labels[githubOrgScopeGCPProjectLabelKey] = config.GetAllowedGithubOrgScopes()
	labels[jobRunsOnGCPProjectLabelKey] = config.GetAllowedJobRunsOn()
	labels[poolLocationGCPProjectLabelKey] = config.GetAllowedPoolLocations()
	labels[poolAvailabilityGCPProjectLabelKey] = config.GetAllowedPoolAvailabilities()
	labels[poolTypeGCPProjectLabelKey] = config.GetAllowedPoolTypes()

	return &RunnerDiscovery{
		cbc:                            cbc,
		aic:                            aic,
		rc:                             rc,
		config:                         config,
		gcpRunnerAllowedProjectLabels:  labels,
		gcpRunnerIgnoredProjectLabels:  config.GetIgnoredGCPProjectLabelsSet(),
		gcpRunnerOptionalProjectLabels: config.GetOptionalGCPProjectLabelsSet(),
	}, nil
}

// Run discovers worker pool projects and caches them in a runner registry.
func (rd *RunnerDiscovery) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	// Fetch all projects in the folder.
	query := generateLabelQuery(rd.gcpRunnerAllowedProjectLabels)
	projects, err := rd.aic.FindProjects(ctx, rd.config.GCPFolderID, query)
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}
	logger.InfoContext(ctx, "Discovered projects from API",
		"query", query,
		"count", len(projects),
		"projects", projects)

	poolsByRegistryKey, err := rd.buildRegistry(ctx, projects)
	if err != nil {
		return fmt.Errorf("failed to build registry: %w", err)
	}

	if err := rd.updateRegistry(ctx, poolsByRegistryKey); err != nil {
		return fmt.Errorf("failed to update registry: %w", err)
	}

	return nil
}

// buildRegistry processes the list of projects to find and group worker pools.
// The key for Redis should be constructed from gh-org-scope and job-runs-on from project labels.
// For example, if gh-org-scope is "default" and job-runs-on is "ubuntu-latest",
// the key will be "default:ubuntu-latest".
func (rd *RunnerDiscovery) buildRegistry(ctx context.Context, projects []*assetinventory.ProjectInfo) (map[string][]registry.WorkerPoolInfo, error) {
	logger := logging.FromContext(ctx)
	poolsByRegistryKey := make(map[string][]registry.WorkerPoolInfo)

	for _, project := range projects {
		projectLabels, ok := rd.filterAndValidateProjectLabels(ctx, project)
		if !ok {
			logger.DebugContext(ctx,
				"Skipping project without target labels",
				"project_id", project.ProjectID,
				"project_labels", projectLabels)
			// A validation error occurred, and the details have been logged. Skip this project.
			continue
		}
		logger.InfoContext(ctx,
			"Checking project for worker pools",
			"project_id", project.ProjectID,
			"project_labels", projectLabels)

		githubOrgScope := projectLabels[githubOrgScopeGCPProjectLabelKey]
		jobRunsOn := projectLabels[jobRunsOnGCPProjectLabelKey]
		location := projectLabels[poolLocationGCPProjectLabelKey]
		poolType := projectLabels[poolTypeGCPProjectLabelKey]

		wps, err := rd.cbc.ListWorkerPools(ctx, project.ProjectID, location)
		if err != nil {
			logger.ErrorContext(ctx,
				"failed to list worker pools",
				"project_id", project.ProjectID,
				"error", err)
			return nil, fmt.Errorf("failed to list worker pools for project %s: %w", project.ProjectID, err)
		}

		if len(wps) == 0 {
			logger.InfoContext(ctx, "no worker pools found in project", "project_id", project.ProjectID)
			continue
		}

		for _, wp := range wps {
			logger.InfoContext(ctx,
				"Found worker pool",
				"project_id", project.ProjectID,
				"worker_pool", wp.GetName(),
				"state", wp.GetState(),
				"config", wp.GetConfig())

			privatePoolConfig := wp.GetPrivatePoolV1Config()
			if privatePoolConfig == nil {
				logger.InfoContext(ctx, "worker pool is not a private pool, skipping", "worker_pool", wp.GetName())
				continue
			}

			workerConfig := privatePoolConfig.GetWorkerConfig()
			if workerConfig == nil {
				logger.InfoContext(ctx, "worker pool has no worker config, skipping", "worker_pool", wp.GetName())
				continue
			}

			registryKey := fmt.Sprintf("%s:%s", githubOrgScope, jobRunsOn)

			// Parse project number and location from the full resource name to ensure accuracy.
			// Format: projects/{PROJECT_NUMBER}/locations/{LOCATION}/workerPools/{WORKERPOOL}
			var poolProjectNumber, poolLocation string
			parts := strings.Split(wp.GetName(), "/")
			if len(parts) == 6 && parts[0] == "projects" && parts[2] == "locations" && parts[4] == "workerPools" {
				poolProjectNumber = parts[1]
				poolLocation = parts[3]
			} else {
				logger.ErrorContext(ctx, "worker pool name is not in expected format, cannot parse project/location", "worker_pool_name", wp.GetName())
				continue // Skip this pool if we can't parse it.
			}

			poolInfo := registry.WorkerPoolInfo{
				Name:          wp.GetName(),
				ProjectID:     project.ProjectID,
				ProjectNumber: poolProjectNumber,
				Location:      poolLocation,
				PoolType:      poolType,
			}
			if val, ok := project.Labels[trustedRemoteConfigGCPProjectLabelKey]; ok {
				poolInfo.RemoteConfig = val
			}
			poolsByRegistryKey[registryKey] = append(poolsByRegistryKey[registryKey], poolInfo)
		}
	}
	return poolsByRegistryKey, nil
}

// updateRegistry handles all interactions with the Redis cache.
func (rd *RunnerDiscovery) updateRegistry(ctx context.Context, poolsByRegistryKey map[string][]registry.WorkerPoolInfo) error {
	logger := logging.FromContext(ctx)

	if rd.rc == nil {
		logger.InfoContext(ctx, "redis client is nil, skipping cache update")
		return nil
	}

	// Sort all collected pools to ensure deterministic order before marshaling.
	for _, pools := range poolsByRegistryKey {
		sort.Slice(pools, func(i, j int) bool {
			return pools[i].Name < pools[j].Name
		})
	}

	// First, prepare all the new data and verify it before touching the cache.
	marshalledPools := make(map[string][]byte)
	for registryKey, pools := range poolsByRegistryKey {
		poolsJSON, err := json.Marshal(pools)
		if err != nil {
			// If we can't marshal the data, we can't update the cache. Abort.
			return fmt.Errorf("failed to marshal pools for key %s: %w", registryKey, err)
		}
		marshalledPools[registryKey] = poolsJSON
	}

	// Find all stale keys that need to be deleted.
	var staleKeys []string
	iter := rd.rc.Scan(ctx, 0, rd.config.RunnerRegistryDefaultKeyPrefix+":*", 0).Iterator()
	for iter.Next(ctx) {
		staleKeys = append(staleKeys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		// If we can't scan, we can't safely update the cache. Abort.
		return fmt.Errorf("failed to scan for stale worker pool keys: %w", err)
	}

	// Atomically delete stale keys and set new keys in a transaction.
	// Only initiate a transaction if there are keys to delete or set.
	if len(staleKeys) == 0 && len(poolsByRegistryKey) == 0 {
		logger.InfoContext(ctx, "no keys to delete or set in registry, skipping transaction")
		return nil
	}

	pipe := rd.rc.TxPipeline()
	if len(staleKeys) > 0 {
		pipe.Del(ctx, staleKeys...)
	}

	// Sort keys to ensure deterministic order for registry SET operations
	sortedKeys := make([]string, 0, len(marshalledPools))
	for key := range marshalledPools {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		value := marshalledPools[key]
		pipe.Set(ctx, key, value, 0)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to execute registry transaction: %w", err)
	}

	// Log the successful cache update.
	for key, value := range marshalledPools {
		logger.InfoContext(ctx,
			"cached worker pools",
			"key", key,
			"value", string(value))
	}

	return nil
}

// validateLabel validates a single label's value against a list of allowed patterns.
// It supports wildcard matching in the allowed patterns.
func (rd *RunnerDiscovery) validateLabel(ctx context.Context, projectID, key, value string, labelAllowedValues []string) bool {
	logger := logging.FromContext(ctx)
	matched := false
	for _, v := range labelAllowedValues {
		match, err := filepath.Match(v, value)
		if err != nil {
			logger.WarnContext(ctx, "invalid wildcard pattern",
				"pattern", v,
				"project_label_value", value,
				"error", err)
			continue
		}
		if match {
			matched = true
			break
		}
	}

	if !matched {
		logger.WarnContext(ctx, fmt.Sprintf("detected unexpected value for label %s, unknown value was %s", key, value),
			"project_id", projectID,
			"label", key,
			"value", value)
		return false
	}
	return true
}

// filterAndValidateProjectLabels validates that a project has all the required
// labels and that the label values are in the allowlist. It also logs a
// warning for any labels that are not in the allowlist. It returns a map of
// the valid labels.
func (rd *RunnerDiscovery) filterAndValidateProjectLabels(ctx context.Context, project *assetinventory.ProjectInfo) (map[string]string, bool) {
	logger := logging.FromContext(ctx)
	projectLabels := make(map[string]string)

	for key, values := range rd.gcpRunnerAllowedProjectLabels {
		projectLabelValue, ok := project.Labels[key]
		if !ok {
			logger.WarnContext(ctx, "project missing required label",
				"project_id", project.ProjectID,
				"label", key)
			return nil, false
		}

		if projectLabelValue == "" {
			logger.WarnContext(ctx, "project has empty label",
				"project_id", project.ProjectID,
				"label", key)
			return nil, false
		}

		if key == poolAvailabilityGCPProjectLabelKey && projectLabelValue != poolAvailabilityAvailable {
			logger.WarnContext(ctx, "pool is unavailable",
				"project_id", project.ProjectID,
				"label", key,
				"project_label_value", projectLabelValue)
			return nil, false
		}

		// We allow for a glob star pattern to accept every label value.
		if !slices.Contains(values, "*") && !rd.validateLabel(ctx, project.ProjectID, key, projectLabelValue, values) {
			return nil, false
		}
		projectLabels[key] = projectLabelValue
	}

	if projectLabels[poolTypeGCPProjectLabelKey] == poolTypeTrusted {
		projectLabelValue, ok := project.Labels[trustedRemoteConfigGCPProjectLabelKey]
		if !ok {
			logger.WarnContext(ctx, "project missing required label because pool-type is trusted",
				"project_id", project.ProjectID,
				"label", trustedRemoteConfigGCPProjectLabelKey)
			return nil, false
		}

		allowedTrustedRemoteConfigs := rd.config.GetAllowedTrustedRemoteConfigs()
		if len(allowedTrustedRemoteConfigs) == 0 {
			logger.WarnContext(ctx, "dispatcher config missing allowed remote config patterns for trusted pools",
				"project_id", project.ProjectID,
				"label", trustedRemoteConfigGCPProjectLabelKey)
			return nil, false
		}

		if !rd.validateLabel(ctx, project.ProjectID, trustedRemoteConfigGCPProjectLabelKey, projectLabelValue, allowedTrustedRemoteConfigs) {
			return nil, false
		}
	}

	// After validating the required labels, iterate through all the project's
	// labels again to log any that are not in the allowlist. This is to
	// alert operators of any unexpected labels that may have been added to
	// a runner project. For example, if a project has a label "foo: bar" and
	// "foo" is not in the allowlist, a warning will be logged.
	for key := range project.Labels {
		_, allowed := rd.gcpRunnerAllowedProjectLabels[key]
		_, optional := rd.gcpRunnerOptionalProjectLabels[key]
		_, ignored := rd.gcpRunnerIgnoredProjectLabels[key]
		if !allowed && !optional && !ignored {
			logger.WarnContext(ctx, "project has non-allowlisted label",
				"project_id", project.ProjectID,
				"label", key)
		}
	}

	return projectLabels, true
}

// generateLabelQuery creates a query for the Cloud Asset API to find projects
// that have all the required labels. The query is a slice of strings, where
// each string is in the format "label_key:*". This will find all projects
// that have the given label key, regardless of the value.
//
// For example, if the allowlist is:
//
//	map[string][]string{
//		"gh-org-scope":  []string{"default"},
//		"job-runs-on": []string{"ubuntu-latest"},
//	}
//
// The generated query will be:
//
//	[]string{"gh-org-scope:*", "job-runs-on:*"}
func generateLabelQuery(allowlist map[string][]string) []string {
	labels := make([]string, 0, len(allowlist))
	for k := range allowlist {
		labels = append(labels, fmt.Sprintf("%s:*", k))
	}
	return labels
}
