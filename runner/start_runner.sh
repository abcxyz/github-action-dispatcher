#!/bin/bash
set -euo pipefail

# Uncompress the Just-In-Time configuration.
ENCODED_JIT_CONFIG="$(echo "${ENCODED_JIT_CONFIG}" | base64 -d | gunzip)"


IDLE_TIMEOUT_SECONDS="${IDLE_TIMEOUT_SECONDS:-300}" # Default to 5 minutes
LOCK_FILE="/tmp/runner.lock"

# Start the runner in the background.
# The DOCKER_HOST variable is inherited from the parent process.
/actions-runner/run.sh --jitconfig "${ENCODED_JIT_CONFIG}" &
RUNNER_PID=$!

echo "Runner started with PID ${RUNNER_PID}. Idle timeout is ${IDLE_TIMEOUT_SECONDS} seconds."

# Start a child process that will kill the runner after the timeout.
(
  sleep "${IDLE_TIMEOUT_SECONDS}"
  if [[ ! -f "${LOCK_FILE}" ]]; then
    echo "Runner has been idle for ${IDLE_TIMEOUT_SECONDS} seconds. Terminating."
    kill "${RUNNER_PID}"
  else
    echo "Runner is busy, idle timeout disabled."
  fi
) &
KILLER_PID=$!

# Wait for the runner to complete a job and exit.
wait "${RUNNER_PID}"

# If the runner exits, it means it ran a job, so we don't need the killer process anymore.
kill "${KILLER_PID}"
# Clean up the lock file on exit
rm -f "${LOCK_FILE}"
