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

	redisapi "github.com/go-redis/redis/v8"

	"github.com/abcxyz/github-action-dispatcher/pkg/assetinventory"
	"github.com/abcxyz/github-action-dispatcher/pkg/cloudbuild"
	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/pkg/logging"
)

const (
	runnerLabelGCPProjectLabelKey    = "runner-label"
	runnerLocationGCPProjectLabelKey = "runner-location"
	runnerTypeGCPProjectLabelKey     = "runner-type"
)

// RunnerDiscovery is the main struct for the runner-discovery job.
type RunnerDiscovery struct {
	cbc    cloudbuild.Client
	aic    assetinventory.Client
	rc     *redisapi.Client
	config *Config
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

	return &RunnerDiscovery{
		cbc:    cbc,
		aic:    aic,
		rc:     rc,
		config: config,
	}, nil
}

// Run discovers worker pool projects and caches them in a runner registry.
func (rd *RunnerDiscovery) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	// Fetch all projects in the folder.
	projects, err := rd.aic.FindProjects(ctx, rd.config.GCPFolderID, rd.config.LabelQuery)
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}
	logger.InfoContext(ctx, "Discovered projects from API",
		"count", len(projects),
		"projects", projects)

	poolsByRegistryKey, err := rd.discoverAndGroupWorkerPools(ctx, projects)
	if err != nil {
		// discoverAndGroupWorkerPools is already logging the error.
		return fmt.Errorf("failed to discover and group worker pools: %w", err)
	}

	if err := rd.updateRegistry(ctx, poolsByRegistryKey); err != nil {
		return fmt.Errorf("failed to update registry: %w", err)
	}

	return nil
}

// discoverAndGroupWorkerPools processes the list of projects to find and group worker pools.
func (rd *RunnerDiscovery) discoverAndGroupWorkerPools(ctx context.Context, projects []*assetinventory.ProjectInfo) (map[string][]registry.WorkerPoolInfo, error) {
	logger := logging.FromContext(ctx)
	poolsByRegistryKey := make(map[string][]registry.WorkerPoolInfo)

	for _, project := range projects {
		logger.InfoContext(ctx,
			"Checking project for worker pools",
			"project_id", project.ProjectID,
			"project_labels", project.Labels)

		runnerType, ok := rd.validateProjectLabel(ctx, project, runnerTypeGCPProjectLabelKey)
		if !ok {
			continue
		}

		runnerLabel, ok := rd.validateProjectLabel(ctx, project, runnerLabelGCPProjectLabelKey)
		if !ok {
			continue
		}

		location, ok := rd.validateProjectLabel(ctx, project, runnerLocationGCPProjectLabelKey)
		if !ok {
			continue
		}

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

			// The key for Redis should be constructed from runner-type and runner-label from project labels.
			registryKey := fmt.Sprintf("%s:%s", runnerType, runnerLabel)
			poolInfo := registry.WorkerPoolInfo{
				Name:      wp.GetName(),
				ProjectID: project.ProjectID,
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

// validateProjectLabel is a helper method to validate the presence and non-empty value of a project label.
func (rd *RunnerDiscovery) validateProjectLabel(ctx context.Context, project *assetinventory.ProjectInfo, labelKey string) (string, bool) {
	logger := logging.FromContext(ctx)

	value, ok := project.Labels[labelKey]
	if !ok {
		logger.WarnContext(ctx, "project missing required label",
			"project_id", project.ProjectID,
			"label", labelKey)
		return "", false
	}
	if value == "" {
		logger.WarnContext(ctx, "project has empty label",
			"project_id", project.ProjectID,
			"label", labelKey)
		return "", false
	}
	return value, true
}
