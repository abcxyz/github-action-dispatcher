#!/bin/bash
set -euo pipefail

# This script acts as a proxy to the real docker binary. It waits for the
# Docker-in-Docker daemon to be ready before executing a command.


# The daemon was already started opportunistically by setup_docker.sh.
# This script's only job is to wait for it to be ready.
if ! /usr/bin/docker.real info > /dev/null 2>&1; then
  echo "Waiting for Docker daemon to initialize..." >&2
  timeout=60
  while ! /usr/bin/docker.real info > /dev/null 2>&1; do
    ((timeout--))
    if [[ "${timeout}" -eq 0 ]]; then
      echo "Timed out waiting for Docker daemon to start." >&2
      exit 1
    fi
    sleep 1
  done
  echo "Docker daemon is ready." >&2
fi

# Now that the daemon is ready, execute the actual docker command
exec /usr/bin/docker.real "$@"
