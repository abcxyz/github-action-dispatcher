#!/bin/bash
set -euo pipefail

_SERVER="${GITHUB_SERVER_URL:-unknown}"
_REPO="${GITHUB_REPOSITORY:-unknown/unknown}"
_RUN="${GITHUB_RUN_ID:-unknown}"
_JOB="${GITHUB_JOB:-unknown}"
GH_JOB_URL="${_SERVER}/${_REPO}/actions/runs/${_RUN}/job/${_JOB}"

echo "==================== BUILD METADATA ===================="
printf "METADATA | JOB_ID: %s | RUN_ID: %s\n" \
  "${GITHUB_JOB:-unknown}" \
  "${GITHUB_RUN_ID:-unknown}"

printf "REPO     | %s\n" \
  "${GITHUB_REPOSITORY:-unknown}"

printf "CONTEXT  | EVENT: %s | ACTOR: %s | REF: %s\n" \
  "${GITHUB_EVENT_NAME:-unknown}" \
  "${GITHUB_TRIGGERING_ACTOR:-unknown}" \
  "${GITHUB_REF_NAME:-unknown}"

printf "WORKFLOW | %s\n" \
  "${GITHUB_WORKFLOW_REF:-unknown}"

printf "LINK     | %s\n" \
  "${GH_JOB_URL}"
echo "========================================================"


LOCK_FILE="/tmp/runner.lock"
touch "${LOCK_FILE}"
echo "Runner lock file created at ${LOCK_FILE}. Idle timeout is now disabled."
