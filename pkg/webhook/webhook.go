// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"time"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/google/go-github/v69/github"

	"github.com/abcxyz/pkg/logging"
)

var (
	defaultRunnerLabel    = "self-hosted"
	runnerStartedMsg      = "runner started"
	githubWebhookEventKey = "github_webhook_event"
)

// apiResponse is a structure that contains a http status code,
// a string response message and any error that might have occurred
// in the processing of a request.
type apiResponse struct {
	Code    int
	Message string
	Error   error
}

func (s *Server) handleWebhook() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := logging.FromContext(ctx)

		resp := s.processRequest(r)
		if resp.Error != nil {
			logger.ErrorContext(ctx, "error processing request",
				"error", resp.Error,
				"code", resp.Code,
				"body", resp.Message)
		}

		w.WriteHeader(resp.Code)
		fmt.Fprint(w, html.EscapeString(resp.Message))
	})
}

func (s *Server) processRequest(r *http.Request) *apiResponse {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	event, err := validateGitHubPayload(r, s.webhookSecret)
	if err != nil {
		logger.ErrorContext(ctx, "failed to validate github payload", "error", err)
		return &apiResponse{http.StatusBadRequest, "failed to validate github payload", err}
	}

	jobID, attributes := extractLoggedAttributes(event)
	logger = logger.With(attributes...)
	// Add to context so attributes are propagated down the stack.
	ctx = logging.WithLogger(ctx, logger)

	switch *event.Action {
	case "queued":
		return s.handleQueuedEvent(ctx, event, jobID)

	case "in_progress":
		if event.WorkflowJob.CreatedAt != nil && event.WorkflowJob.StartedAt != nil {
			queuedDuration := event.WorkflowJob.StartedAt.Sub(event.WorkflowJob.CreatedAt.Time)
			logger = logger.With("duration_queued_seconds", queuedDuration.Seconds())
		}

		logger.InfoContext(ctx, "Workflow job in progress")
		return &apiResponse{http.StatusOK, "workflow job in progress event logged", nil}

	case "completed":
		logger.InfoContext(ctx, "Workflow job completed", extractCompletedLogAttributes(event)...)
		return &apiResponse{http.StatusOK, "workflow job completed event logged", nil}

	default:
		// Log other unhandled workflow job actions
		logger.InfoContext(ctx, "no action taken for unhandled workflow job action type", "action", *event.Action)
		return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for action type: %q", *event.Action), nil}
	}
}

func validateGitHubPayload(r *http.Request, webhookSecret []byte) (*github.WorkflowJobEvent, error) {
	payload, err := github.ValidatePayload(r, webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to validate payload: %w", err)
	}

	rawEvent, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse webhook: %w", err)
	}

	event, ok := rawEvent.(*github.WorkflowJobEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected event type dispatched from webhook, event type: %T", rawEvent)
	}

	// Validate event object
	var merr error
	if event.Action == nil {
		merr = errors.Join(merr, fmt.Errorf("event is missing required field: action"))
	}

	// A workflow job is required for all actions.
	if event.WorkflowJob == nil {
		merr = errors.Join(merr, fmt.Errorf("event is missing required field: workflow_job"))
	}
	if merr != nil {
		return nil, merr
	}
	return event, nil
}

func (s *Server) handleQueuedEvent(ctx context.Context, event *github.WorkflowJobEvent, jobID string) *apiResponse {
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "Workflow job queued")

	var label string
	// We don't support jobs with multiple labels.
	if len(event.WorkflowJob.Labels) != 1 {
		logger.WarnContext(ctx, "no action taken, only accept single label jobs", "labels", event.WorkflowJob.Labels)
		return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken, only accept single label jobs, got: %s", event.WorkflowJob.Labels), nil}
	} else if s.runnerLabel == event.WorkflowJob.Labels[0] {
		label = s.runnerLabel
	} else if s.enableSelfHostedLabel && event.WorkflowJob.Labels[0] == "self-hosted" {
		label = "self-hosted"
	}

	if label == "" {
		logger.WarnContext(ctx, "no action taken for label", "labels", event.WorkflowJob.Labels)
		return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for label: %s", event.WorkflowJob.Labels), nil}
	}

	if event.Installation == nil || event.Installation.ID == nil || event.Org == nil || event.Org.Login == nil || event.Repo == nil || event.Repo.Name == nil {
		err := fmt.Errorf("event is missing required fields (installation, org, or repo)")
		logger.ErrorContext(ctx, "cannot generate JIT config due to missing event data", "error", err)
		return &apiResponse{http.StatusBadRequest, "unexpected event payload struture", err}
	}

	for i := 1; i <= 1+s.extraRunnerCount; i++ {
		runnerID := fmt.Sprintf("GCP-%s-%d", jobID, i)
		logger := logger.With("runner_id", runnerID) // Shadow logger per runner.
		if i > 1 {
			logger.InfoContext(ctx, "Spawning extra runner")
		}
		ctx = logging.WithLogger(ctx, logger)

		responseText, err := s.startGitHubRunner(ctx, event, runnerID, logger, s.runnerImageTag, label)
		if err != nil {
			return &apiResponse{http.StatusInternalServerError, responseText, err}
		}

		logger.InfoContext(ctx, runnerStartedMsg, slog.Any(githubWebhookEventKey, event))
	}

	return &apiResponse{http.StatusOK, runnerStartedMsg, nil}
}

func extractLoggedAttributes(event *github.WorkflowJobEvent) (string, []any) {
	// Common attributes to always include for WorkflowJobEvent
	var jobID, runID, jobName string
	if event.WorkflowJob.ID != nil {
		jobID = fmt.Sprintf("%d", *event.WorkflowJob.ID)
	}
	if event.WorkflowJob.RunID != nil {
		runID = fmt.Sprintf("%d", *event.WorkflowJob.RunID)
	}
	if event.WorkflowJob.Name != nil {
		jobName = *event.WorkflowJob.Name
	}

	// Base log fields that will be common to most WorkflowJob logs
	attributes := []any{
		"action_event_name", *event.Action,
		"gh_run_id", runID,
		"gh_job_id", jobID,
		"gh_job_name", jobName,
		"job_id", jobID,
	}

	// Add all available timestamps to logger attributes (they might be nil depending on event action)
	if event.WorkflowJob.CreatedAt != nil {
		attributes = append(attributes, "created_at", getTimeString(event.WorkflowJob.CreatedAt))
	}
	if event.WorkflowJob.StartedAt != nil {
		attributes = append(attributes, "started_at", getTimeString(event.WorkflowJob.StartedAt))
	}
	if event.WorkflowJob.CompletedAt != nil {
		attributes = append(attributes, "completed_at", getTimeString(event.WorkflowJob.CompletedAt))
	}
	return jobID, attributes
}

func extractCompletedLogAttributes(event *github.WorkflowJobEvent) []any {
	var completedAttributes []any
	if event.WorkflowJob.Conclusion != nil {
		completedAttributes = append(completedAttributes, "conclusion", *event.WorkflowJob.Conclusion)
	}

	if event.WorkflowJob.StartedAt != nil && event.WorkflowJob.CompletedAt != nil {
		inProgressDuration := event.WorkflowJob.CompletedAt.Sub(event.WorkflowJob.StartedAt.Time)
		completedAttributes = append(completedAttributes, "duration_in_progress_seconds", inProgressDuration.Seconds())
	}

	// Optional: Also log total duration from creation to completion here
	if event.WorkflowJob.CreatedAt != nil && event.WorkflowJob.CompletedAt != nil {
		totalDuration := event.WorkflowJob.CompletedAt.Sub(event.WorkflowJob.CreatedAt.Time)
		completedAttributes = append(completedAttributes, "duration_total_seconds", totalDuration.Seconds())
	}
	return completedAttributes
}

func compressAndBase64EncodeString(input string) (string, error) {
	var compressedJIT bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&compressedJIT, gzip.BestCompression)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip writer: %w", err)
	}
	_, err = gzipWriter.Write([]byte(input))
	if err != nil {
		return "", fmt.Errorf("failed to write to gzip writer: %w", err)
	}
	err = gzipWriter.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}
	return base64.StdEncoding.EncodeToString(compressedJIT.Bytes()), nil
}

func (s *Server) startGitHubRunner(ctx context.Context, event *github.WorkflowJobEvent, runnerID string, logger *slog.Logger, imageTag string, runnerLabel string) (string, error) {
	jitConfig, err := s.GenerateRepoJITConfig(ctx, *event.Installation.ID, *event.Org.Login, *event.Repo.Name, runnerID, runnerLabel)
	if err != nil {
		logger.ErrorContext(ctx, "failed to generate JIT config",
			"error", err.Error(),
		)
		return "error generating jitconfig", err
	}

	// Sometimes JITConfig has exceeded the 4,000-character limit for
	// substitutions. It has nested base64 encoded data, so it is very
	// compressible.
	compressedJIT, err := compressAndBase64EncodeString(*jitConfig.EncodedJITConfig)
	if err != nil {
		return "failed to compress JIT config", err
	}

	build := &cloudbuildpb.Build{
		ServiceAccount: s.runnerServiceAccount,
		Steps: []*cloudbuildpb.BuildStep{
			{
				Id:   "run",
				Name: "$_REPOSITORY_ID/$_IMAGE_NAME:$_IMAGE_TAG",
				Env: []string{
					"ENCODED_JIT_CONFIG=${_ENCODED_JIT_CONFIG}",
				},
			},
		},
		Options: &cloudbuildpb.BuildOptions{
			Logging: cloudbuildpb.BuildOptions_CLOUD_LOGGING_ONLY,
		},
		Substitutions: map[string]string{
			"_ENCODED_JIT_CONFIG": compressedJIT,
			"_REPOSITORY_ID":      s.runnerRepositoryID,
			"_IMAGE_NAME":         s.runnerImageName,
			"_IMAGE_TAG":          imageTag,
		},
	}

	// Check if this is an E2E test run and add appropriate tags.
	if s.e2eTestRunID != "" {
		build.Tags = []string{"e2e-test", fmt.Sprintf("e2e-run-id-%s", s.e2eTestRunID)}
	}

	if s.runnerWorkerPoolID != "" {
		build.Options.Pool = &cloudbuildpb.BuildOptions_PoolOption{
			Name: s.runnerWorkerPoolID,
		}
	}

	buildReq := &cloudbuildpb.CreateBuildRequest{
		Parent:    fmt.Sprintf("projects/%s/locations/%s", s.runnerProjectID, s.runnerLocation),
		ProjectId: s.runnerProjectID,
		Build:     build,
	}

	if err := s.cbc.CreateBuild(ctx, buildReq); err != nil {
		err = fmt.Errorf("failed to create cloud run build: %w", err)
		logger.ErrorContext(ctx, "cloud run build failed", "error", err)
		return "failed to create build", err
	}
	return "", nil
}

// getTimeString is a helper function to format a *github.Timestamp pointer into an ISO 8601 string.
// It safely handles nil *github.Timestamp pointers.
// It returns "N/A" if the time pointer is nil.
func getTimeString(ghTime *github.Timestamp) string {
	if ghTime == nil { // ONLY check if the *pointer* itself is nil
		return "N/A"
	}
	return ghTime.Format(time.RFC3339Nano)
}
