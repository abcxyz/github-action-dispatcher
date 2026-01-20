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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/testutil"
)

func TestNewConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		lookup envconfig.Lookuper
		expCfg *RegistryConfig
		expErr string
	}{
		{
			name: "success",
			lookup: envconfig.MapLookuper(map[string]string{
				"REGISTRY_HOST": "localhost",
				"REGISTRY_PORT": "6379",
			}),
			expCfg: &RegistryConfig{
				Host: "localhost",
				Port: "6379",
			},
		},
		{
			name: "missing_host",
			lookup: envconfig.MapLookuper(map[string]string{
				"REGISTRY_PORT": "6379",
			}),
			expErr: `failed to parse registry config: failed to load config: Host: missing required value: REGISTRY_HOST`,
		},
		{
			name: "missing_port",
			lookup: envconfig.MapLookuper(map[string]string{
				"REGISTRY_HOST": "localhost",
			}),
			expErr: `failed to parse registry config: failed to load config: Port: missing required value: REGISTRY_PORT`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotCfg, err := newConfig(t.Context(), tc.lookup)
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}

			if diff := cmp.Diff(tc.expCfg, gotCfg); diff != "" {
				t.Errorf("Config unexpected diff (-want,+got):\n%s", diff)
			}
		})
	}
}
