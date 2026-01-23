// Copyright 2025 Google LLC
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

package webhook

import (
	"fmt"
)

type ReadFileResErr struct {
	Res []byte
	Err error
}

type MockFileReader struct {
	ReadFileMock *ReadFileResErr
	ReadFileFunc func(filename string) ([]byte, error)
}

func (m *MockFileReader) ReadFile(filename string) ([]byte, error) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(filename)
	}

	// Fallback to ReadFileMock if ReadFileFunc is not provided
	if m.ReadFileMock != nil {
		return m.ReadFileMock.Res, m.ReadFileMock.Err
	}
	return nil, fmt.Errorf("mock ReadFile not implemented")
}
