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

package retry

import (
	"time"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
)

// BackoffConfig defines the configuration for a retry backoff strategy.
type BackoffConfig struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
}

// DefaultBackoffConfig returns a BackoffConfig with sensible defaults.
func DefaultBackoffConfig() *BackoffConfig {
	return &BackoffConfig{
		Initial:    500 * time.Millisecond,
		Max:        60 * time.Second,
		Multiplier: 2.0,
	}
}

// NewGRPCCallOption returns a gRPC call option that configures a retry
// mechanism for gRPC calls.
func NewGRPCCallOption(cfg *BackoffConfig) gax.CallOption {
	return gax.WithRetry(func() gax.Retryer {
		return gax.OnCodes([]codes.Code{
			codes.Unavailable,
			codes.DeadlineExceeded,
			codes.ResourceExhausted,
		}, gax.Backoff{
			Initial:    cfg.Initial,
			Max:        cfg.Max,
			Multiplier: cfg.Multiplier,
		})
	})
}
