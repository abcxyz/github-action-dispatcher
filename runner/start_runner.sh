#!/bin/bash
set -euo pipefail

#!/bin/bash
set -euo pipefail

# Uncompress the Just-In-Time configuration.
ENCODED_JIT_CONFIG="$(echo "${ENCODED_JIT_CONFIG}" | base64 -d | gunzip)"

# Start the runner. The DOCKER_HOST variable is inherited from the parent process.
/actions-runner/run.sh --jitconfig $ENCODED_JIT_CONFIG &
wait $!
