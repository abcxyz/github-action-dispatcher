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

package redis

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
)

// NewClient creates and returns a new Redis client.
// It uses the host and port from the provided config.
func NewClient(ctx context.Context, cfg *Config) (*redis.Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("redis host must be provided")
	}
	if cfg.Port == "" {
		return nil, fmt.Errorf("redis port must be provided")
	}

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return rdb, nil
}
