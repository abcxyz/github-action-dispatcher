#!/bin/bash
set -euo pipefail

# Use this to find the current hash of cached images.
# docker image ls --digests

# Check: can the current user ('root') access Docker without sudo?
if docker info > /dev/null 2>&1; then
    echo "SUCCESS: Docker daemon is responsive to the 'root' user."
else
    echo "ERROR: 'docker info' as 'runner' user (UID $(id -u || true)) failed."
    exit 1
fi

# Uncompress jitconfig
ENCODED_JIT_CONFIG="$(echo "${ENCODED_JIT_CONFIG}" | base64 -d | gunzip)"

# Finally register a github runner using the jit config env variable.
/workspace/actions-runner/run.sh --jitconfig "${ENCODED_JIT_CONFIG:?}" &
wait $!
