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

// Package webhook is the base webhook server for a github app's events specific to queued workflow jobs.

package webhook

import (
	"fmt"
	"github.com/abcxyz/pkg/logging"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) handleDeleteRunner() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := logging.FromContext(ctx)

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "missing authorization header")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(s.runnerJWTSecret), nil
		})
		if err != nil {
			logger.ErrorContext(ctx, "failed to parse jwt", "error", err)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "invalid token")
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			runnerIDFloat, ok := claims["runner_id"].(float64)
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "invalid runner_id claim")
				return
			}
			runnerID := int64(runnerIDFloat)

			org, ok := claims["org"].(string)
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "invalid org claim")
				return
			}

			installation, err := s.appClient.InstallationForOrg(ctx, org)
			if err != nil {
				logger.ErrorContext(ctx, "failed to get installation for org", "error", err, "org", org)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "failed to get installation")
				return
			}

			httpClient := oauth2.NewClient(ctx, (*installation).AllReposOAuth2TokenSource(ctx, map[string]string{
				"administration": "write",
			}))

			gh := github.NewClient(httpClient)
			baseURL, err := url.Parse(fmt.Sprintf("%s/", s.ghAPIBaseURL))
			if err != nil {
				logger.ErrorContext(ctx, "failed to parse base url", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "internal server error")
				return
			}
			gh.BaseURL = baseURL
			gh.UploadURL = baseURL

			if repo, repoOk := claims["repo"].(string); repoOk && repo != "" {
				_, err = gh.Actions.RemoveRunner(ctx, org, repo, runnerID)
			} else {
				_, err = gh.Actions.RemoveOrganizationRunner(ctx, org, runnerID)
			}

			if err != nil {
				logger.ErrorContext(ctx, "failed to remove runner", "error", err, "runner_id", runnerID)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "failed to remove runner")
				return
			}

			logger.InfoContext(ctx, "successfully deleted runner", "runner_id", runnerID)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "runner deleted")
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "invalid token")
		}
	})
}
