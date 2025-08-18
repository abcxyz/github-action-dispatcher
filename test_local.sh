#!/usr/bin/env bash

set -euo pipefail

# Build runner image
docker build -t runner:latest ./runner

RUNNER_NAME="local-$(tr -dc A-Za-z0-9 </dev/urandom | head -c 12 || true)"

echo "Generating JIT Config. If no more output, something failed. Try running \
command without storing to variable to see output."

#shellcheck disable=SC1091 # local.env should not be checked in
if [[ -f env/local.env ]]; then
  set -o allexport
  source env/local.env
  set +o allexport
fi

# shellcheck disable=2154 # referenced vars are env vars
# Generate JIT Config
JIT_CONFIG="$(
    go run ./cmd/generate-jit \
        -app-id "${GH_APP_ID}" \
        -private-key "${GH_APP_PEM_PATH}" \
        -org "${GH_ORG_NAME}" \
        -runner-name "${RUNNER_NAME}" \
        -runner-group-id 1 \
        | gzip | base64
)"

echo "I haven't found a way to make it interruptable once the runner is \
listening. Use docker ps and docker kill in another terminal window."

# Run runner with jitconfig
docker run -it --privileged -e ENCODED_JIT_CONFIG="${JIT_CONFIG}" runner:latest
