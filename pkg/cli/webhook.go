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
	"net/http"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/github-action-dispatcher/pkg/version"
	"github.com/abcxyz/github-action-dispatcher/pkg/webhook"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
	"github.com/abcxyz/pkg/serving"
)

var _ cli.Command = (*WebhookServerCommand)(nil)

type WebhookServerCommand struct {
	cli.BaseCommand

	cfg         *webhook.Config
	registryCfg *registry.RegistryConfig

	// only used for testing
	testFlagSetOpts          []cli.Option
	testWebhookClientOptions *webhook.WebhookClientOptions
}

func (c *WebhookServerCommand) Desc() string {
	return `Start a webhook server for github-action-dispatcher`
}

func (c *WebhookServerCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options]
  Start a webhook server for github-action-dispatcher.
`
}

func (c *WebhookServerCommand) Flags() *cli.FlagSet {
	c.cfg = &webhook.Config{}
	c.registryCfg = &registry.RegistryConfig{}
	set := cli.NewFlagSet(c.testFlagSetOpts...)
	c.cfg.ToFlags(set)
	c.registryCfg.ToFlags(set)
	return set
}

func (c *WebhookServerCommand) Run(ctx context.Context, args []string) error {
	server, mux, err := c.RunUnstarted(ctx, args)
	if err != nil {
		return err
	}

	return server.StartHTTPHandler(ctx, mux)
}

func (c *WebhookServerCommand) Process(
	ctx context.Context,
	h *renderer.Renderer,
	cfg *webhook.Config,
	registryClient *redis.Client,
	gcbHTTPClient *http.Client,
	webhookClientOptions *webhook.WebhookClientOptions,
) (*serving.Server, http.Handler, error) {
	webhookServer, err := webhook.NewServer(ctx, h, cfg, registryClient, gcbHTTPClient, webhookClientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server: %w", err)
	}

	mux := webhookServer.Routes(ctx)

	server, err := serving.New(cfg.Port)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create serving infrastructure: %w", err)
	}

	return server, mux, nil
}

func (c *WebhookServerCommand) RunUnstarted(ctx context.Context, args []string) (*serving.Server, http.Handler, error) {
	f := c.Flags()
	if err := f.Parse(args); err != nil {
		return nil, nil, fmt.Errorf("failed to parse flags: %w", err)
	}
	args = f.Args()
	if len(args) > 0 {
		return nil, nil, fmt.Errorf("unexpected arguments: %q", args)
	}

	logger := logging.FromContext(ctx)
	logger.DebugContext(ctx, "server starting",
		"name", version.Name,
		"commit", version.Commit,
		"version", version.Version)

	h, err := renderer.New(ctx, nil,
		renderer.WithOnError(func(err error) {
			logger.ErrorContext(ctx, "failed to render", "error", err)
		}))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create renderer: %w", err)
	}

	if err := c.cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}
	logger.DebugContext(ctx, "loaded configuration", "config", c.cfg)

	registryClient, err := registry.NewRunnerRegistry(ctx, c.registryCfg)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create registry client, caching will be disabled", "error", err)
	}

	gcbHTTPClient, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create default google client: %w", err)
	}

	var webhookClientOptions *webhook.WebhookClientOptions
	if c.testWebhookClientOptions != nil {
		webhookClientOptions = c.testWebhookClientOptions
	} else {
		agent := fmt.Sprintf("google:github-action-dispatcher/%s", version.Version)
		opts := []option.ClientOption{option.WithUserAgent(agent)}
		webhookClientOptions = &webhook.WebhookClientOptions{
			KeyManagementClientOpts: opts,
		}
	}

	return c.Process(ctx, h, c.cfg, registryClient, gcbHTTPClient, webhookClientOptions)
}
