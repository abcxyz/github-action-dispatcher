# Docker Bootstrapping Information

## Background
Google Cloud Build mounts its docker daemon to the image the runner runs on. 
This would enable Docker-Out-Of-Docker (DooD), where container actions or
docker commands within actions would be started using the host's docker daemon.

Docker-Out-Of-Docker runs into the issue where any paths mounted reference the 
host's filesystem, rather than the path from the runner. This could cause us to
run into compatibility issues. Some of these can be fixed by changing the GitHub
workspace to be mounted in /workspace as this is a volume which is mounted in
both the GCB VM and the runner's container, but doesn't provide a general
solution. See [commit 9218a14](https://github.com/abcxyz/github-action-dispatcher/commit/9218a14cb8b038b3a80e7506f7a8349c156f48e2)
for a working example.

## Solution

Because Docker-Out-Of-Docker could cause compatibility issues, Docker-In-Docker
(DinD) was used. It initially wasn't seen as a good option due to the long
startup time of the Docker daemon, which must be paid even if an action doesn't
use docker.

The solution was a bit of a hack: start the Docker daemon in the background and
continue starting the GitHub runner. Replace the `docker` binary with a proxy
script which waits for the Docker daemon to start and then passes on commands.

This has three main parts:
1. `docker_proxy.sh`: this is placed in `/usr/bin/docker`. It will catch any
calls to Docker and ensure the Daemon is started before passing to the real
`Docker` binary.
2. `INTERNAL_DOCKER_SOCKET`: Since the host's docker daemon is already mounted,
we need to start our Docker socket at a different location than the default.
3. `dockerd-entrypoint.sh`: This is a script provided by Docker for using DinD.
We always start the daemon in the background, so if Docker is needed you don't
have to pay the entire startup cost.

With this in place, workflows that don't use Docker don't block on the daemon
starting. Those that do often will see the daemon has already started by the
time they would need to block.
