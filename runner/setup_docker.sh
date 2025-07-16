#!/bin/bash
set -euo pipefail

# Docker-in-Docker Path Translation Workaround:
# The Cloud Build environment provides a shared `/workspace` directory that is visible
# to both this runner container and the host's Docker daemon.
# By moving the entire actions-runner directory into `/workspace`, we ensure that
# the runner's $GITHUB_WORKSPACE variable will always resolve to a path under `/workspace`.
# This allows docker volume mounts (-v $GITHUB_WORKSPACE:...) to work correctly,
# as the source path is visible to the Docker daemon.
mv /actions-runner /workspace/

RUNNER_PATH="/workspace/actions-runner"

# Give the runner ownership of its new directory.
chown -R runner:runner "${RUNNER_PATH}"

# Get the Group ID (GID) of the mounted docker socket
DOCKER_SOCKET_GID=$(stat -c '%g' /var/run/docker.sock)

# 🛡️ SECURITY CHECK: Refuse to proceed if the socket's group is root (GID 0).
# Since the runner does have full sudo permissions, this won't stop a malicious
# actor.
if [[ "${DOCKER_SOCKET_GID}" -eq 0 ]]; then
  echo "!! DANGER !!" >&2
  echo "Docker socket is owned by the 'root' group (GID 0)." >&2
  echo "Adding a user to the root group is a major security risk." >&2
  echo "Aborting." >&2
  exit 1
fi

# Create a group with the socket's GID if it doesn't already exist.
getent group "${DOCKER_SOCKET_GID}" || groupadd --gid "${DOCKER_SOCKET_GID}" docker_socket

# Add the 'runner' user to the docker socket's group.
usermod -aG "${DOCKER_SOCKET_GID}" runner

#    Then, drop privileges and execute the startup script.
exec gosu runner "${RUNNER_PATH}/start_runner.sh" "$@"
