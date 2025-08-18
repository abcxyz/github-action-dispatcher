#!/bin/bash
set -euo pipefail

echo "--- Checking for privileged access... ---"
# Create a temporary directory to use as a safe mountpoint for our test.
MOUNT_TEST_DIR="$(mktemp -d)"

# Create an isolated mount namespace (-m)
# and then attempt the 'mount' operation. This will only succeed if the
# container has the required capabilities (like CAP_SYS_ADMIN) AND is not
# blocked by a host security policy (like AppArmor/SELinux).
if unshare -m -- mount -t tmpfs tmpfs "${MOUNT_TEST_DIR}" &>/dev/null; then
    echo "SUCCESS: Container has sufficient privileges to run Docker-in-Docker."
    rm -rf -- "${MOUNT_TEST_DIR}"
else
    echo "ERROR: Container lacks the necessary mount permissions for DinD." >&2
    echo "Ensure the container is started with the --privileged flag." >&2
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
