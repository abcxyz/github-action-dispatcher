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

	"github.com/abcxyz/pkg/logging"
)

const (
	runnerLabelGCPProjectLabelKey    = "runner-label"
	runnerLocationGCPProjectLabelKey = "runner-location"
	runnerTypeGCPProjectLabelKey     = "runner-type"
)

// RunnerDiscovery is the main struct for the runner-discovery job.
type RunnerDiscovery struct {
	cbc    cloudBuildClient
	aic    assetInventoryClient
	rc     *redisapi.Client
	config *Config
}

// NewRunnerDiscovery creates a new RunnerDiscovery instance.
// It initializes the necessary Cloud Build and Asset Inventory clients based on the provided configuration,
// and accepts a registry client for caching.
// Returns a pointer to the initialized RunnerDiscovery instance or an error if client creation fails.
func NewRunnerDiscovery(ctx context.Context, config *Config, rc *redisapi.Client) (*RunnerDiscovery, error) {
	cbc, err := newCloudBuildClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud build client: %w", err)
	}

	aic, err := newAssetInventoryClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud asset inventory client: %w", err)
	}

	return &RunnerDiscovery{
		cbc:    cbc,
		aic:    aic,
		rc:     rc,
		config: config,
	}, nil
}

// Run is the main entrypoint for the runner-discovery job.
// It discovers projects based on configured criteria, lists worker pools within those projects,
// and logs relevant information. Returns an error if any part of the discovery process fails.
func (rd *RunnerDiscovery) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	logger.InfoContext(ctx, "Calling Run method")

	projects, err := rd.aic.Projects(ctx, rd.config.GCPFolderID, rd.config.LabelQuery)
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}
	logger.InfoContext(ctx, "Discovered projects from API",
		"count", len(projects),
		"projects", projects)

	poolsByRegistryKey := make(map[string][]string)
	for _, project := range projects {
		logger.InfoContext(ctx,
			"Checking project for worker pools",
			"project_id", project.ID,
			"project_number", project.Number,
			"project_labels", project.Labels)

		runnerType, ok := project.Labels[runnerTypeGCPProjectLabelKey]
		if !ok {
			logger.WarnContext(ctx, "project missing required label",
				"project_id", project.ID,
				"label", runnerTypeGCPProjectLabelKey)
			continue
		}
		if runnerType == "" {
			logger.WarnContext(ctx, "project has empty runner-type label", "project_id", project.ID)
			continue
		}

		runnerLabel, ok := project.Labels[runnerLabelGCPProjectLabelKey]
		if !ok {
			logger.WarnContext(ctx, "project missing required label",
				"project_id", project.ID,
				"label", runnerLabelGCPProjectLabelKey)
			continue
		}
		if runnerLabel == "" {
			logger.WarnContext(ctx, "project has empty runner-label", "project_id", project.ID)
			continue
		}

		location, ok := project.Labels[runnerLocationGCPProjectLabelKey]
		if !ok {
			logger.WarnContext(ctx, "project missing required label",
				"project_id", project.ID,
				"label", runnerLocationGCPProjectLabelKey)
			continue
		}
		if location == "" {
			logger.WarnContext(ctx, "project has empty runner-location label", "project_id", project.ID)
			continue
		}

		wps, err := rd.cbc.ListWorkerPools(ctx, project.ID, location)
		if err != nil {
			return fmt.Errorf("failed to list worker pools for project %s: %w", project.ID, err)
		}

		if len(wps) == 0 {
			logger.InfoContext(ctx, "no worker pools found in project", "project_id", project.ID)
			continue
		}

		for _, wp := range wps {
			logger.InfoContext(ctx,
				"Found worker pool",
				"project_id", project.ID,
				"project_number", project.Number,
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
			poolsByRegistryKey[registryKey] = append(poolsByRegistryKey[registryKey], wp.GetName())
		}
	}

	// Sort all collected pools to ensure deterministic order before marshaling.
	for _, pools := range poolsByRegistryKey {
		sort.Strings(pools)
	}

	if rd.rc == nil {
		return nil
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
