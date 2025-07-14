#!/bin/bash
set -euo pipefail

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

# Switch to the 'runner' user and execute the startup script.
exec gosu runner /actions-runner/start_runner.sh "$@"
