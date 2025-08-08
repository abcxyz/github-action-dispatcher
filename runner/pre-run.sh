#!/bin/bash
set -euo pipefail

LOCK_FILE="/tmp/runner.lock"
touch "${LOCK_FILE}"
echo "Runner lock file created at ${LOCK_FILE}. Idle timeout is now disabled."
