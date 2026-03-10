// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package assetinventory

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFindProjects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		gcpFolderID string
		labelQuery  []string
		wantQuery   string
	}{
		{
			name:        "nil label query",
			gcpFolderID: "123456789",
			labelQuery:  nil,
			wantQuery:   `state="ACTIVE"`,
		},
		{
			name:        "empty label query",
			gcpFolderID: "123456789",
			labelQuery:  []string{},
			wantQuery:   `state="ACTIVE"`,
		},
		{
			name:        "with label query",
			gcpFolderID: "123456789",
			labelQuery:  []string{"foo:bar"},
			wantQuery:   `state="ACTIVE" AND labels.foo:bar`,
		},
		{
			name:        "with multiple label query",
			gcpFolderID: "123456789",
			labelQuery:  []string{"foo:bar", "abc:xyz"},
			wantQuery:   `state="ACTIVE" AND labels.foo:bar AND labels.abc:xyz`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			client := &MockClient{}
			_, err := client.FindProjects(ctx, tc.gcpFolderID, tc.labelQuery)
			if err != nil {
				t.Errorf("FindProjects() error = %v", err)
			}
			if diff := cmp.Diff(tc.wantQuery, client.gotQuery); diff != "" {
				t.Errorf("FindProjects() query (-want +got): %s", diff)
			}
		})
	}
}
