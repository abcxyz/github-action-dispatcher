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

package httpclient

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/abcxyz/github-action-dispatcher/pkg/retry"
)

// NewRetryableRoundTripper returns a custom http.RoundTripper that retries requests with exponential backoff.
// It uses the provided BackoffConfig for retry parameters and a fixed number of retries.
func NewRetryableRoundTripper(transport http.RoundTripper, cfg *retry.BackoffConfig, maxRetries int) http.RoundTripper {
	return &retryableRoundTripper{
		transport:  transport,
		cfg:        cfg,
		maxRetries: maxRetries,
	}
}

// retryableRoundTripper is a custom http.RoundTripper that retries requests with exponential backoff.
type retryableRoundTripper struct {
	transport  http.RoundTripper
	cfg        *retry.BackoffConfig
	maxRetries int
}

// RoundTrip implements the http.RoundTripper interface.
func (rt *retryableRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error
	for i := 0; i <= rt.maxRetries; i++ { // Loop up to maxRetries attempts
		resp, err := rt.transport.RoundTrip(req)
		if err == nil && resp.StatusCode < 500 { // Success if no error and status code is not a server error
			return resp, nil
		}

		lastErr = err
		if resp != nil {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		if i < rt.maxRetries { // Only sleep if there are more retries
			// Calculate exponential backoff duration
			delay := float64(rt.cfg.Initial) * math.Pow(rt.cfg.Multiplier, float64(i))
			sleepDuration := time.Duration(delay)
			if sleepDuration > rt.cfg.Max {
				sleepDuration = rt.cfg.Max
			}
			time.Sleep(sleepDuration)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", rt.maxRetries, lastErr)
}