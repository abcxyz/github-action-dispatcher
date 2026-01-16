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
	"testing"

	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/sethvargo/go-envconfig"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		cfg    *Config
		expErr string
	}{
		{
			name: "valid",
			cfg: &Config{
				Host: "localhost",
				Port: "6379",
			},
		},
		{
			name: "missing_host",
			cfg: &Config{
				Port: "6379",
			},
			expErr: "REDIS_HOST must be provided",
		},
		{
			name: "missing_port",
			cfg: &Config{
				Host: "localhost",
			},
			expErr: "REDIS_PORT must be provided",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		lookup envconfig.Lookuper
		expCfg *Config
		expErr string
	}{
		{
			name: "success",
			lookup: envconfig.MapLookuper(map[string]string{
				"REDIS_HOST": "localhost",
				"REDIS_PORT": "6379",
			}),
			expCfg: &Config{
				Host: "localhost",
				Port: "6379",
			},
		},
		{
			name: "missing_host",
			lookup: envconfig.MapLookuper(map[string]string{
				"REDIS_PORT": "6379",
			}),
			expErr: `failed to parse redis config: failed to load config: Host: missing required value: REDIS_HOST`,
		},
		{
			name: "missing_port",
			lookup: envconfig.MapLookuper(map[string]string{
				"REDIS_HOST": "localhost",
			}),
			expErr: `failed to parse redis config: failed to load config: Port: missing required value: REDIS_PORT`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotCfg, err := newConfig(context.Background(), tc.lookup)
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}

			if diff := cmp.Diff(tc.expCfg, gotCfg); diff != "" {
				t.Errorf("Config unexpected diff (-want,+got):\n%s", diff)
			}
		})
	}
}
