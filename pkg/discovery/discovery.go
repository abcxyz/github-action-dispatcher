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
	locationLabel    = "runner-location"
	fallbackLocation = "us-central1"
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
	logger.InfoContext(ctx, "Discovered projects from API", "projects", projects)

	poolsByMachineType := make(map[string][]string)
	for _, project := range projects {
		logger.InfoContext(ctx,
			"Checking project for worker pools",
			"project_id", project.ID,
			"project_number", project.Number,
			"project_labels", project.Labels)

		location, ok := project.Labels[locationLabel]
		if !ok {
			location = fallbackLocation
		}

		wps, err := rd.cbc.ListWorkerPools(ctx, project.ID, location)
		if err != nil {
			logger.ErrorContext(ctx,
				"failed to list worker pools",
				"project_id", project.ID,
				"project_number", project.Number,
				"error", err)
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

			machineType := workerConfig.GetMachineType()
			if machineType == "" {
				logger.InfoContext(ctx, "worker pool has no machine type, skipping", "worker_pool", wp.GetName())
				continue
			}
			poolsByMachineType[machineType] = append(poolsByMachineType[machineType], wp.GetName())
		}
	}

	if rd.rc == nil {
		return nil
	}

	// First, prepare all the new data and verify it before touching the cache.
	marshalledPools := make(map[string][]byte)
	for machineType, pools := range poolsByMachineType {
		registryKey := fmt.Sprintf("default-%s", machineType)
		poolsJSON, err := json.Marshal(pools)
		if err != nil {
			// If we can't marshal the data, we can't update the cache. Abort.
			return fmt.Errorf("failed to marshal pools for machine type %s: %w", machineType, err)
		}
		marshalledPools[registryKey] = poolsJSON
	}

	// Find all stale keys that need to be deleted.
	var staleKeys []string
	iter := rd.rc.Scan(ctx, 0, "default-*", 0).Iterator()
	for iter.Next(ctx) {
		staleKeys = append(staleKeys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		// If we can't scan, we can't safely update the cache. Abort.
		return fmt.Errorf("failed to scan for stale worker pool keys: %w", err)
	}

	// Atomically delete stale keys and set new keys in a transaction.
	// Only initiate a transaction if there are keys to delete or set.
	if len(staleKeys) == 0 && len(marshalledPools) == 0 {
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
