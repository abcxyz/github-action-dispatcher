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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"

	"github.com/abcxyz/pkg/logging"
)

const (
	deprecatedSelfHostedRunnerLabel   = "self-hosted"
	runnerStartedMsg                  = "runner started"
	githubWebhookEventKey             = "github_webhook_event"
	selfHostedUbuntuLatestRunnerLabel = "sh-ubuntu-latest"
)

// apiResponse is a structure that contains a http status code,
// a string response message and any error that might have occurred
// in the processing of a request.
type apiResponse struct {
	Code    int
	Message string
	Error   error
}

type runnersResponse struct {
	Message     string   `json:"message"`
	RunnerNames []string `json:"runnerNames"`
}

type runnerRequest struct {
	JITConfig string `json:"jitConfig"`
	Label     string `json:"label"`
	RunnerID  string `json:"runnerId"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
}

type jitConfigPayload struct {
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	Labels []string `json:"labels"`
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

		// If the response is a JSON object, set the correct content type.
		if strings.HasPrefix(resp.Message, "{") {
			w.Header().Set("Content-Type", "application/json")
		}

		w.WriteHeader(resp.Code)

		fmt.Fprint(w, resp.Message)
	})
}

func (s *Server) handleJITConfig() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := logging.FromContext(ctx)

		resp := s.processJITConfigRequest(r)
		if resp.Error != nil {
			logger.ErrorContext(ctx, "error processing jit config request",
				"error", resp.Error,
				"code", resp.Code,
				"body", resp.Message)
		}

		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(resp.Code)

		fmt.Fprint(w, resp.Message)
	})
}

var validateIAPToken = func(ctx context.Context, token, audience string) error {
	if token == "" {
		return fmt.Errorf("missing IAP token")
	}
	payload, err := idtoken.Validate(ctx, token, audience)
	if err != nil {
		return fmt.Errorf("failed to validate IAP token: %w", err)
	}
	if payload.Audience != audience {
		return fmt.Errorf("audit mismatch: got %q, want %q", payload.Audience, audience)
	}
	return nil
}

func (s *Server) processJITConfigRequest(r *http.Request) *apiResponse {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	iapToken := r.Header.Get("x-goog-iap-jwt-assertion")
	if err := validateIAPToken(ctx, iapToken, s.iapServiceAudience); err != nil {
		return &apiResponse{http.StatusForbidden, "invalid IAP token", err}
	}

	var payload jitConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return &apiResponse{http.StatusBadRequest, "invalid request body", err}
	}

	if payload.Owner == "" || payload.Repo == "" || len(payload.Labels) == 0 {
		return &apiResponse{http.StatusBadRequest, "owner, repo, and labels are required", fmt.Errorf("missing required fields")}
	}

	if !s.isAllowed(payload.Owner, payload.Repo, payload.Labels) {
		logger.WarnContext(ctx, "jit config request denied by allowlist",
			"owner", payload.Owner,
			"repo", payload.Repo,
			"labels", payload.Labels,
		)
		return &apiResponse{http.StatusForbidden, "request denied by allowlist", fmt.Errorf("denied by allowlist")}
	}

	// 4. Generate JIT Config
	installationID := s.installationID

	runnerName := uuid.New().String()
	jitConfig, err := s.GenerateRepoJITConfig(ctx, installationID, payload.Owner, payload.Repo, runnerName, payload.Labels)
	if err != nil {
		return &apiResponse{http.StatusInternalServerError, "failed to generate JIT config", err}
	}

	responseBytes, err := json.Marshal(jitConfig)
	if err != nil {
		return &apiResponse{http.StatusInternalServerError, "failed to marshal response", err}
	}

	return &apiResponse{http.StatusOK, string(responseBytes), nil}
}

func (s *Server) isAllowed(owner, repo string, labels []string) bool {
	// If allowlist is empty/nil, DENY ALL (default secure).
	if len(s.jitConfigAllowlist) == 0 {
		return false
	}

	// Check Owner
	repoMap, ok := s.jitConfigAllowlist[owner]
	if !ok {
		// Check for wildcard owner "*"
		repoMap, ok = s.jitConfigAllowlist["*"]
		if !ok {
			return false
		}
	}

	// Check Repo
	allowedLabels, ok := repoMap[repo]
	if !ok {
		// Check for wildcard repo "*"
		allowedLabels, ok = repoMap["*"]
		if !ok {
			return false
		}
	}

	// Check Labels
	for _, reqLabel := range labels {
		allowed := false
		for _, allowLabel := range allowedLabels {
			if allowLabel == "*" || allowLabel == reqLabel {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
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

// startRunnersForJob contains the core logic for spawning runners for a given
// queued job. It returns the names of the runners it successfully started and
// an error if anything went wrong.
func (s *Server) startRunnersForJob(ctx context.Context, event *github.WorkflowJobEvent, label string) ([]string, error) {
	logger := logging.FromContext(ctx)

	// This slice will hold the names of runners we successfully create.
	var startedRunnerNames []string

	for i := 1; i <= 1+s.extraRunnerCount; i++ {
		runnerID := uuid.New().String()

		runnerLogger := logger.With("runner_id", runnerID)
		if i > 1 {
			runnerLogger.InfoContext(ctx, "Spawning extra runner")
		}

		runnerCtx := logging.WithLogger(ctx, runnerLogger)

		responseText, err := s.startGitHubRunner(runnerCtx, event, runnerID, runnerLogger, label)
		if err != nil {
			// If one fails, return the error and the list of any that succeeded before it.
			return startedRunnerNames, fmt.Errorf("failed on runner %s: %w. response: %s", runnerID, err, responseText)
		}

		runnerLogger.InfoContext(ctx, runnerStartedMsg, slog.Any(githubWebhookEventKey, event))
		startedRunnerNames = append(startedRunnerNames, runnerID)
	}

	return startedRunnerNames, nil
}

func (s *Server) handleQueuedEvent(ctx context.Context, event *github.WorkflowJobEvent, jobID string) *apiResponse {
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "Workflow job queued")

	// We don't support jobs with multiple labels.
	if len(event.WorkflowJob.Labels) != 1 {
		logger.WarnContext(ctx, "no action taken, only accept single label jobs", "labels", event.WorkflowJob.Labels)
		return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken, only accept single label jobs, got: %s", event.WorkflowJob.Labels), nil}
	}

	incomingLabel := event.WorkflowJob.Labels[0]
	var labelToUse string

	if incomingLabel == s.runnerLabel || incomingLabel == selfHostedUbuntuLatestRunnerLabel {
		labelToUse = s.runnerLabel
	} else if s.enableSelfHostedLabel && incomingLabel == deprecatedSelfHostedRunnerLabel {
		// This case is a temporary hack to allow us to migrate away from the self-hosted label.
		// It should be deleted once that is done.
		labelToUse = deprecatedSelfHostedRunnerLabel
	}

	if labelToUse == "" {
		logger.WarnContext(ctx, "no action taken for label", "labels", event.WorkflowJob.Labels)
		return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for label: %s", event.WorkflowJob.Labels), nil}
	}

	if event.Installation == nil || event.Installation.ID == nil || event.Org == nil || event.Org.Login == nil || event.Repo == nil || event.Repo.Name == nil {
		err := fmt.Errorf("event is missing required fields (installation, org, or repo)")
		logger.ErrorContext(ctx, "cannot generate JIT config due to missing event data", "error", err)
		return &apiResponse{http.StatusBadRequest, "unexpected event payload struture", err}
	}

	runnerNames, err := s.startRunnersForJob(ctx, event, labelToUse)
	if err != nil {
		return &apiResponse{http.StatusInternalServerError, err.Error(), err}
	}

	responsePayload := &runnersResponse{
		Message:     runnerStartedMsg,
		RunnerNames: runnerNames,
	}

	// Marshal the struct into a JSON string.
	responseBytes, err := json.Marshal(responsePayload)
	if err != nil {
		return &apiResponse{http.StatusInternalServerError, "failed to serialize response", err}
	}

	return &apiResponse{http.StatusOK, string(responseBytes), nil}
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

func (s *Server) startGitHubRunner(ctx context.Context, event *github.WorkflowJobEvent, runnerID string, logger *slog.Logger, runnerLabel string) (string, error) {
	jitConfig, err := s.GenerateRepoJITConfig(ctx, *event.Installation.ID, *event.Org.Login, *event.Repo.Name, runnerID, []string{runnerLabel})
	if err != nil {
		logger.ErrorContext(ctx, "failed to generate JIT config",
			"error", err.Error(),
		)
		return "error generating jitconfig", err
	}

	jitEncoded := *jitConfig.EncodedJITConfig

	reqBody := &runnerRequest{
		JITConfig: jitEncoded,
		Label:     runnerLabel,
		RunnerID:  runnerID,
		Owner:     *event.Org.Login,
		Repo:      *event.Repo.Name,
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal runner request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.externalRunnerEndpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// We might need auth for the external endpoint, but not specified in requirements yet.
	// Assuming it's an internal or protected endpoint, or we should add auth?
	// User only mentioned payload. I'll stick to basic post for now.

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send runner creation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("external endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	return "success", nil
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
