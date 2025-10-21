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
EXIT_CODE=0
wait "${RUNNER_PID}" || EXIT_CODE=$?

# If the runner exits, it means it ran a job, so we don't need the killer process anymore.
# It's possible the killer process has already exited (in the timeout case),
# so we add `|| true` to prevent `set -e` from failing the script.
kill "${KILLER_PID}" || true
# Clean up the lock file on exit
rm -f "${LOCK_FILE}"

# Exit with a success code if the runner was terminated by the idle timeout.
# Exit code 143 corresponds to SIGTERM.
if [[ "${EXIT_CODE}" == "143" || "${EXIT_CODE}" == "0" ]]; then
  echo "Runner exited with code ${EXIT_CODE}. Assuming success."
  exit 0
else
  echo "Runner exited with unexpected code ${EXIT_CODE}. Failing."
  exit "${EXIT_CODE}"
fi
