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

package registry

import (
	"context"
	"fmt"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/cfgloader"
	"github.com/abcxyz/pkg/cli"
)

// RegistryConfig defines the set of environment variables required
// for creating a registry client.
type RegistryConfig struct {
	Host string `env:"REDIS_HOST,required"`
	Port string `env:"REDIS_PORT,required"`
}

// ToFlags binds the config to a CLI flag set.
func (c *RegistryConfig) ToFlags(set *cli.FlagSet) {
	f := set.NewSection("Registry Options")
	f.StringVar(&cli.StringVar{
		Name:   "redis-host",
		Target: &c.Host,
		EnvVar: "REDIS_HOST",
		Usage:  "The host of the redis server.",
	})
	f.StringVar(&cli.StringVar{
		Name:   "redis-port",
		Target: &c.Port,
		EnvVar: "REDIS_PORT",
		Usage:  "The port of the redis server.",
	})
}

// NewConfig creates a new RegistryConfig from environment variables.
func NewConfig(ctx context.Context) (*RegistryConfig, error) {
	return newConfig(ctx, envconfig.OsLookuper())
}

func newConfig(ctx context.Context, lu envconfig.Lookuper) (*RegistryConfig, error) {
	var cfg RegistryConfig
	if err := cfgloader.Load(ctx, &cfg, cfgloader.WithLookuper(lu)); err != nil {
		return nil, fmt.Errorf("failed to parse registry config: %w", err)
	}
	return &cfg, nil
}
