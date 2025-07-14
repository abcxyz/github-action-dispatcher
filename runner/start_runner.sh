#!/bin/bash
set -e

# Use this to find the current hash of cached images.
# docker image ls --digests

# Check: can the current user ('root') access Docker without sudo?
if docker info > /dev/null 2>&1; then
    echo "SUCCESS: Docker daemon is responsive to the 'root' user."
else
    echo "ERROR: 'docker info' as 'root' user (UID $(id -u)) failed."
    exit 1
fi

# This ensures Docker CLI commands (i.e. login) run by actions
# will use a writable location for their configuration.
export HOME="/home/runner"
export DOCKER_CONFIG="${HOME}/.docker-runner-default"

# Create the directory if it doesn't exist.
# Since this script runs as the 'runner' user, 'runner' will own this directory.
mkdir -p "${DOCKER_CONFIG}"
echo "Default DOCKER_CONFIG for this runner session set to: ${DOCKER_CONFIG}"

# Uncompress jitconfig
ENCODED_JIT_CONFIG="$(echo "${ENCODED_JIT_CONFIG}" | base64 -d | gunzip)"

# Finally register a github runner using the jit config env variable.
/actions-runner/run.sh --jitconfig $ENCODED_JIT_CONFIG &
wait $!
