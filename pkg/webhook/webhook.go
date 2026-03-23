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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-github/v69/github"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/abcxyz/github-action-dispatcher/pkg/registry"
	"github.com/abcxyz/pkg/logging"
)

const (
	runnerStartedMsg      = "runner started"
	githubWebhookEventKey = "github_webhook_event"
	gcbBuildIDKey         = "gcb_build_id"
	gcbProjectIDKey       = "gcb_project_id"
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
	GCBBuildIDs []string `json:"gcbBuildIDs,omitempty"`
}

// handleWebhook returns an http.Handler that processes incoming GitHub webhook requests.
//
// It decodes the webhook payload, validates it, and dispatches the event to the appropriate handler
// based on the event type and action.
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

// processRequest handles the core logic of processing an incoming HTTP request
// believed to be a GitHub webhook.
//
// It validates the payload, extracts relevant information, and delegates
// to specific handlers based on the workflow job event action.
// It returns an apiResponse containing the HTTP status, message, and any error encountered.
func (s *Server) processRequest(r *http.Request) *apiResponse {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	event, err := validateGitHubPayload(r, s.webhookSecret)
	if err != nil {
		logger.ErrorContext(ctx, "failed to validate github payload", "error", err)
		return &apiResponse{http.StatusBadRequest, "failed to validate github payload", err}
	}
	if event == nil {
		return &apiResponse{http.StatusOK, "ignored event", nil}
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

// validateGitHubPayload validates the incoming HTTP request as a GitHub webhook payload.
//
// It uses `github.ValidatePayload` and `github.ParseWebHook` to ensure the payload is authentic
// and correctly formatted. It specifically expects a `github.WorkflowJobEvent`. Other event types
// like `InstallationRepositoriesEvent` or `InstallationEvent` are logged and ignored.
// It returns the parsed `github.WorkflowJobEvent` or an error if validation fails or the event type is unexpected.
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
		switch rawEvent.(type) {
		case *github.InstallationRepositoriesEvent, *github.InstallationEvent:
			// These are specific event types (like installation events) that are expected but do not require further processing, so we log and ignore them.
			slog.InfoContext(r.Context(), "received event", "type", github.WebHookType(r))
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected event type dispatched from webhook, event type: %T", rawEvent)
		}
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
func (s *Server) startRunnersForJob(ctx context.Context, event *github.WorkflowJobEvent, jobOriginalRunnerLabel, jobResolvedRunnerLabel string) ([]string, []string, error) {
	logger := logging.FromContext(ctx)

	// These slices will hold the names and build IDs of runners we successfully create.
	var startedRunnerNames []string
	var gcbBuildIDs []string

	pool := s.selectWorkerPool(ctx, jobResolvedRunnerLabel)

	for i := 1; i <= 1+s.extraRunnerCount; i++ {
		runnerID := uuid.New().String()

		runnerLogger := logger.With("runner_id", runnerID)
		if i > 1 {
			runnerLogger.InfoContext(ctx, "Spawning extra runner")
		}

		runnerCtx := logging.WithLogger(ctx, runnerLogger)
		buildID, projectID, err := s.startGitHubRunner(runnerCtx, event, runnerID, runnerLogger, s.runnerImageName, s.runnerImageTag, jobOriginalRunnerLabel, pool)
		if err != nil {
			// If one fails, return the error and the list of any that succeeded before it.
			return startedRunnerNames, gcbBuildIDs, fmt.Errorf("failed on runner %s: %w", runnerID, err)
		}

		runnerLogger.InfoContext(ctx, runnerStartedMsg,
			slog.Any(githubWebhookEventKey, event),
			slog.String(gcbBuildIDKey, buildID),
			slog.String(gcbProjectIDKey, projectID))

		startedRunnerNames = append(startedRunnerNames, runnerID)
		gcbBuildIDs = append(gcbBuildIDs, buildID)
	}

	return startedRunnerNames, gcbBuildIDs, nil
}

// start404RunnerForJob starts a runner for the 404 runner.
func (s *Server) start404RunnerForJob(ctx context.Context, event *github.WorkflowJobEvent, jobOriginalRunnerLabel string) ([]string, []string, error) {
	logger := logging.FromContext(ctx)

	runnerID := uuid.New().String()
	runnerLogger := logger.With("runner_id", runnerID)
	runnerCtx := logging.WithLogger(ctx, runnerLogger)

	// Use the default fallback pool.
	var noPool *registry.WorkerPoolInfo
	buildID, projectID, err := s.startGitHubRunner(runnerCtx, event, runnerID, runnerLogger, s.config.Runner404ImageName, s.config.Runner404ImageTag, jobOriginalRunnerLabel, noPool)
	if err != nil {
		return nil, nil, fmt.Errorf("failed on runner %s: %w", runnerID, err)
	}

	runnerLogger.InfoContext(ctx, runnerStartedMsg,
		slog.Any(githubWebhookEventKey, event),
		slog.String(gcbBuildIDKey, buildID),
		slog.String(gcbProjectIDKey, projectID))

	return []string{runnerID}, []string{buildID}, nil
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
	jobOriginalRunnerLabel := incomingLabel // used in jit config request

	logger.InfoContext(ctx, "received user requested label", "label", incomingLabel)

	jobResolvedRunnerLabel, canHandle, err := s.resolveAndValidateRunnerLabel(ctx, incomingLabel)
	if err != nil {
		logger.ErrorContext(ctx, "failed to resolve and validate runner label", "error", err)
		return &apiResponse{http.StatusInternalServerError, err.Error(), err}
	}

	if !canHandle && !s.config.Runner404Enabled {
		logger.WarnContext(ctx, "no action taken for label",
			"original_label", jobOriginalRunnerLabel,
			"resolved_label", jobResolvedRunnerLabel)
		return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for label: %s", incomingLabel), nil}
	}

	if event.Installation == nil || event.Installation.ID == nil || event.Org == nil || event.Org.Login == nil || event.Repo == nil || event.Repo.Name == nil {
		err := fmt.Errorf("event is missing required fields (installation, org, or repo)")
		logger.ErrorContext(ctx, "cannot generate JIT config due to missing event data", "error", err)
		return &apiResponse{http.StatusBadRequest, "unexpected event payload struture", err}
	}

	var runnerNames, gcbBuildIDs []string
	if !canHandle && s.config.Runner404Enabled {
		// This assumes that the dispatcher is responsible for enqueuing all
		// jobs on the GH host. If another service will subscribe to the
		// webhook and handle jobs then this should not be enabled.
		runnerNames, gcbBuildIDs, err = s.start404RunnerForJob(ctx, event, jobOriginalRunnerLabel)
		if err != nil {
			return &apiResponse{http.StatusInternalServerError, err.Error(), err}
		}
	} else {
		runnerNames, gcbBuildIDs, err = s.startRunnersForJob(ctx, event, jobOriginalRunnerLabel, jobResolvedRunnerLabel)
		if err != nil {
			return &apiResponse{http.StatusInternalServerError, err.Error(), err}
		}
	}

	responsePayload := &runnersResponse{
		Message:     runnerStartedMsg,
		RunnerNames: runnerNames,
		GCBBuildIDs: gcbBuildIDs,
	}

	// Marshal the struct into a JSON string.
	responseBytes, err := json.Marshal(responsePayload)
	if err != nil {
		return &apiResponse{http.StatusInternalServerError, "failed to serialize response", err}
	}

	return &apiResponse{http.StatusOK, string(responseBytes), nil}
}

// resolveAndValidateRunnerLabel encapsulates the logic for resolving runner labels
// and checking if they are provisionable based on the server's configuration.
func (s *Server) resolveAndValidateRunnerLabel(ctx context.Context, incomingLabel string) (string, bool, error) {
	logger := logging.FromContext(ctx)

	logger.DebugContext(ctx, "RunnerLabelAliases map content", "aliases", s.config.RunnerLabelAliases)

	// Determine the lookup label for the worker pool after resolving aliases.
	jobResolvedRunnerLabel := incomingLabel
	visited := make(map[string]bool)
	for {
		if visited[jobResolvedRunnerLabel] {
			err := fmt.Errorf("detected alias cycle for label %q", incomingLabel)
			logger.ErrorContext(ctx, err.Error(), "label", incomingLabel)
			return "", false, err
		}
		visited[jobResolvedRunnerLabel] = true

		if next, ok := s.config.RunnerLabelAliases[jobResolvedRunnerLabel]; ok {
			logger.InfoContext(ctx, "resolved runner label alias",
				"original_label", jobResolvedRunnerLabel,
				"resolved_label", next)
			jobResolvedRunnerLabel = next
		} else {
			break
		}
	}

	// Check if the jobResolvedRunnerLabel is in the combined allowlist.
	canHandle := s.allowedLabels[jobResolvedRunnerLabel]

	return jobResolvedRunnerLabel, canHandle, nil
}

// getRunnerKey creates a key for the runner in the format that the registry
// expects. This key is used to lookup additional information about the runner,
// such as the worker pool to use.
func (s *Server) getRunnerKey(ctx context.Context, label string) string {
	// TODO(controllers): Add support for org-specific keys.
	// For now, we only support default runners that are org agnostic.
	return fmt.Sprintf("%s:%s", s.runnerRegistryDefaultKeyPrefix, label)
}

// extractLoggedAttributes extracts common logging attributes from a GitHub WorkflowJobEvent.
//
// It populates a slice of key-value pairs suitable for structured logging, including
// job ID, run ID, job name, and various timestamps if available.
// It returns the jobID as a string and the slice of logging attributes.
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

	if event.Repo != nil && event.Repo.HTMLURL != nil && event.WorkflowJob != nil && event.WorkflowJob.RunID != nil {
		workflowRunURL := fmt.Sprintf("%s/actions/runs/%d", *event.Repo.HTMLURL, *event.WorkflowJob.RunID)
		attributes = append(attributes, "workflow_run_url", workflowRunURL)
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

// extractCompletedLogAttributes extracts logging attributes specific to a completed GitHub WorkflowJobEvent.
//
// It includes the conclusion of the workflow job and calculates the duration
// of the job in progress and total duration from creation to completion.
// It returns a slice of logging attributes.
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

// compressAndBase64EncodeString compresses the input string using gzip
// and then encodes the compressed data into a base64 string.
//
// This is used to reduce the size of the JIT config for Cloud Build substitutions.
// It returns the base64 encoded string or an error if compression or encoding fails.
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

// startGitHubRunner generates a JIT config and creates a Cloud Build job to start a GitHub runner.
//
// It takes the GitHub WorkflowJobEvent, a unique runner ID, logger, image tag, runner label, and pool.
// It returns the build ID and project ID on success, or an error if the JIT config generation or Cloud Build
// job creation fails.
func (s *Server) startGitHubRunner(ctx context.Context, event *github.WorkflowJobEvent, runnerID string, logger *slog.Logger, imageName, imageTag, jobOriginalRunnerLabel string, pool *registry.WorkerPoolInfo) (string, string, error) {
	compressedJIT, err := s.generateAndCompressJITConfig(ctx, event, runnerID, jobOriginalRunnerLabel)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate and compress JIT config: %w", err)
	}

	buildReq := s.buildCloudBuildRequest(ctx, compressedJIT, imageName, imageTag, pool)

	buildID, err := s.cbc.CreateBuild(ctx, buildReq)
	if err != nil {
		return "", "", fmt.Errorf("failed to create build: %w", err)
	}
	return buildID, buildReq.GetProjectId(), nil
}

// generateAndCompressJITConfig handles the logic of generating and compressing the JIT config.
func (s *Server) generateAndCompressJITConfig(ctx context.Context, event *github.WorkflowJobEvent, runnerID, jobOriginalRunnerLabel string) (string, error) {
	logger := logging.FromContext(ctx)
	jitConfig, err := s.ghc.GenerateRepoJITConfig(ctx, *event.Installation.ID, *event.Org.Login, *event.Repo.Name, runnerID, jobOriginalRunnerLabel)
	if err != nil {
		logger.ErrorContext(ctx, "failed to generate JIT config", "error", err)
		return "", fmt.Errorf("error generating jitconfig: %w", err)
	}

	// Sometimes JITConfig has exceeded the 4,000-character limit for
	// substitutions. It has nested base64 encoded data, so it is very
	// compressible.
	compressedJIT, err := compressAndBase64EncodeString(*jitConfig.EncodedJITConfig)
	if err != nil {
		return "", fmt.Errorf("failed to compress JIT config: %w", err)
	}
	return compressedJIT, nil
}

// selectWorkerPool selects a worker pool for the job. It returns a specific
// *registry.WorkerPoolInfo if a pool is found in the registry, otherwise it
// returns nil.
func (s *Server) selectWorkerPool(ctx context.Context, jobResolvedRunnerLabel string) *registry.WorkerPoolInfo {
	logger := logging.FromContext(ctx)
	pools := s.getWorkerPools(ctx, jobResolvedRunnerLabel)

	if len(pools) > 0 {
		// Use random selection to select a pool.
		randomIndex := rand.Intn(len(pools)) //nolint:gosec // G404: Cryptographic randomness is not required for worker pool selection.
		selectedPool := pools[randomIndex]
		logger.InfoContext(
			ctx,
			"found worker pool in registry",
			"worker_pool", selectedPool.Name,
			"total_worker_pools_found", len(pools),
		)
		return &selectedPool
	}

	// Fallback to default.
	logger.InfoContext(
		ctx,
		"no worker pools found in registry for label, using default server configuration",
		"label", jobResolvedRunnerLabel,
	)
	return nil
}

// buildCloudBuildRequest creates a cloud build request.
func (s *Server) buildCloudBuildRequest(ctx context.Context, compressedJIT, imageName, imageTag string, pool *registry.WorkerPoolInfo) *cloudbuildpb.CreateBuildRequest {
	logger := logging.FromContext(ctx)
	build := &cloudbuildpb.Build{
		Timeout: durationpb.New(time.Duration(s.runnerExecutionTimeoutSeconds) * time.Second),
		Steps: []*cloudbuildpb.BuildStep{
			{
				Id:   "run",
				Name: "$_REPOSITORY_ID/$_IMAGE_NAME:$_IMAGE_TAG",
				Env: []string{
					"ENCODED_JIT_CONFIG=${_ENCODED_JIT_CONFIG}",
					"IDLE_TIMEOUT_SECONDS=${_IDLE_TIMEOUT_SECONDS}",
					"CREATE_BUILD_REQUEST_TIME_UTC=${_CREATE_BUILD_REQUEST_TIME_UTC}",
				},
			},
		},
		Options: &cloudbuildpb.BuildOptions{
			Logging: cloudbuildpb.BuildOptions_CLOUD_LOGGING_ONLY,
		},
		Substitutions: map[string]string{
			"_ENCODED_JIT_CONFIG":            compressedJIT,
			"_IDLE_TIMEOUT_SECONDS":          strconv.Itoa(s.runnerIdleTimeoutSeconds),
			"_REPOSITORY_ID":                 s.runnerRepositoryID,
			"_IMAGE_NAME":                    imageName,
			"_IMAGE_TAG":                     imageTag,
			"_CREATE_BUILD_REQUEST_TIME_UTC": time.Now().UTC().Format(time.RFC3339),
		},
	}

	// Check if this is an E2E test run and add appropriate tags.
	if s.e2eTestRunID != "" {
		build.Tags = []string{"e2e-test", fmt.Sprintf("e2e-run-id-%s", s.e2eTestRunID)}
	}

	var projectID, location, serviceAccount string

	if pool != nil && pool.ProjectID != "" {
		// Use configuration from the registry.
		projectID = pool.ProjectID
		// Use location from pool, but fall back to server default if it's missing
		// for safety during transitions.
		location = pool.Location
		if location == "" {
			location = s.runnerLocation
			logger.WarnContext(ctx, "worker pool from registry is missing location, falling back to default",
				"pool_name", pool.Name,
				"default_location", location)
		}

		serviceAccount = fmt.Sprintf("runner-sa@%s.iam.gserviceaccount.com", pool.ProjectID)
		build.Options.Pool = &cloudbuildpb.BuildOptions_PoolOption{Name: pool.Name}
	} else if s.config.Runner404Enabled {
		// If enabled, use the 404 runner to fail the job with no assigned pool.
		projectID = s.config.Runner404ProjectID
		location = s.config.Runner404Location
		serviceAccount = s.config.Runner404ServiceAccount
		if s.config.Runner404WorkerPoolID != "" {
			build.Options.Pool = &cloudbuildpb.BuildOptions_PoolOption{Name: s.Runner404WorkerPoolID}
		}
	} else {
		// Otherwise, use the default server configuration.
		projectID = s.runnerProjectID
		location = s.runnerLocation
		serviceAccount = s.runnerServiceAccount
		if s.runnerWorkerPoolID != "" {
			build.Options.Pool = &cloudbuildpb.BuildOptions_PoolOption{Name: s.runnerWorkerPoolID}
		}
	}

	// Ensure the service account is in the full resource name format.
	if serviceAccount != "" && !strings.HasPrefix(serviceAccount, "projects/") {
		serviceAccount = fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, serviceAccount)
	}

	build.ServiceAccount = serviceAccount

	return &cloudbuildpb.CreateBuildRequest{
		Parent:    fmt.Sprintf("projects/%s/locations/%s", projectID, location),
		ProjectId: projectID,
		Build:     build,
	}
}

// getWorkerPools determines the appropriate worker pools for a given runner label.
func (s *Server) getWorkerPools(ctx context.Context, runnerLabel string) []registry.WorkerPoolInfo {
	logger := logging.FromContext(ctx)

	// Return nil if registry is not configured.
	if s.rc == nil {
		logger.DebugContext(ctx, "registry not configured, no worker pools found")
		return nil
	}

	// Attempt to get pools from the registry.
	poolsKey := s.getRunnerKey(ctx, runnerLabel)
	logger.DebugContext(ctx, "attempting to get worker pool from registry", "key", poolsKey)
	val, err := s.rc.Get(ctx, poolsKey).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			// If it's a real error, log it, but we can still fall back to the default.
			logger.ErrorContext(ctx, "Redis GET operation failed for worker pool from registry",
				"error", err,
				"key", poolsKey)
		}
		logger.DebugContext(ctx, "worker pool not found in registry (or redis.Nil error), using default",
			"key", poolsKey,
			"worker_pool", s.runnerWorkerPoolID)
		return nil
	}
	logger.DebugContext(ctx, "successfully retrieved worker pool from registry", "key", poolsKey)

	var pools []registry.WorkerPoolInfo
	if err := json.Unmarshal([]byte(val), &pools); err != nil {
		logger.ErrorContext(ctx, "failed to unmarshal pools from registry",
			"error", err,
			"value", val)
		return nil
	}
	logger.DebugContext(ctx, "retrieved worker pools from registry", "pools", pools)

	if len(pools) == 0 {
		logger.InfoContext(ctx, "no worker pools found in registry for label, using default",
			"label", runnerLabel,
			"worker_pool", s.runnerWorkerPoolID)
		return nil
	}

	return pools
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
