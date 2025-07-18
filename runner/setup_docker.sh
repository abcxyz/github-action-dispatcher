#!/bin/bash
set -euo pipefail

# --- Privilege Check ---
echo "--- Checking for privileged access... ---"
# Check if any of the common host block devices exist.
if [ -e "/dev/sda" ] || [ -e "/dev/vda" ] || [ -e "/dev/xvda" ]; then
    echo "SUCCESS: Privileged access confirmed."
else
    echo "ERROR: Container is not running in privileged mode." >&2
    echo "This Docker-in-Docker setup requires the --privileged flag to function." >&2
    exit 1
fi

# --- Opportunistic DinD Start ---
# Define a non-conflicting path for the DinD socket to avoid collision with the host's mount
INTERNAL_DOCKER_SOCKET="unix:///var/run/docker-internal.sock"

# Set the DOCKER_HOST variable so all docker commands talk to our new DinD daemon
export DOCKER_HOST="${INTERNAL_DOCKER_SOCKET}"

# Start the daemon in the background on the new socket
# Forcing --storage-driver=vfs is crucial for reliability in Cloud Build
dockerd-entrypoint.sh dockerd --host="${INTERNAL_DOCKER_SOCKET}" --storage-driver=vfs &

# Drop privileges and execute the runner startup script
# The DOCKER_HOST variable will be passed to the runner's environment
exec gosu runner /actions-runner/start_runner.sh "$@"
