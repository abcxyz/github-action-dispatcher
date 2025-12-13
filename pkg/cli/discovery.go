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

package cli

import (
	"context"
	"fmt"

	"github.com/abcxyz/github-action-dispatcher/pkg/discovery"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

var _ cli.Command = (*RunnerDiscoveryCommand)(nil)

type RunnerDiscoveryCommand struct {
	cli.BaseCommand
}

func (c *RunnerDiscoveryCommand) Desc() string {
	return `Execute the runner-discovery job`
}

func (c *RunnerDiscoveryCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options]

Execute the runner-discovery job to find projects available as runners.`
}

func (c *RunnerDiscoveryCommand) Flags() *cli.FlagSet {
	return cli.NewFlagSet()
}

func (c *RunnerDiscoveryCommand) Run(ctx context.Context, args []string) error {
	logger := logging.FromContext(ctx)
	cfg, err := discovery.NewConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	logger.DebugContext(ctx, "loaded configuration", "config", cfg)

	rd, err := discovery.NewRunnerDiscovery(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create runner discovery: %w", err)
	}

	if err := rd.Run(ctx); err != nil {
		return fmt.Errorf("failed to run runner discovery: %w", err)
	}
	return nil
}
