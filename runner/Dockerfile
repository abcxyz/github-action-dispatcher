# This cached image has an old version of Ubuntu which isn't compatible with
# some moderdern software. At the expense of performance we will re-create it
# with a more modern version of Ubuntu until it can be fixed.
# FROM gcr.io/cloud-builders/docker@sha256:057d91e53efd3e025350d09ecf0fea30ec261bdc5ef5e052d1351c6c6d9dfe21
# ========================================================== #
# Cloud-builders image but with updated ubuntu version BEGIN #
# ========================================================== #
# This is current cached version, I would prefer to use
# marketplace.gcr.io/google/ubuntu2404 but it doesn't appear to be in the cache.
# I believe it tracks latest so we would need to update the hash ocassionally
# if we wanted to use this long-term.
FROM ubuntu@sha256:a08e551cb33850e4740772b38217fc1796a66da2506d312abe51acda354ff061

# I had to add tini because it is no longer included with docker-ce package
# (was getting failures from dockerd-entrypoint due to missing docker-init).
RUN apt-get -y update && \
    apt-get -y install \
        apt-transport-https \
        ca-certificates \
        curl \
        make \
        software-properties-common \
        tini && \
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | \
        gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) \
        signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] \
        https://download.docker.com/linux/ubuntu \
        $(lsb_release -cs) stable" > /etc/apt/sources.list.d/docker.list && \
    # tini doesn't add a docker-init symlink, must do manually \
    ln -s /usr/bin/tini /usr/local/bin/docker-init && \
    apt-get -y update && \
    apt-get -y dist-upgrade && \
    apt-get autoremove && \
    apt-get clean

# This is a newer version of docker than in the cloud-builder image, due to
# that version not existing in apt repo for 24.04. This should be fine.
ARG DOCKER_VERSION=5:28.3.3-1~ubuntu.24.04~noble

RUN apt-get -y install \
        docker-ce=${DOCKER_VERSION} \
        docker-ce-cli=${DOCKER_VERSION} \
        docker-compose docker-compose-plugin && \
    apt-get clean

# ======================================================== #
# Cloud-builders image but with updated ubuntu version END #
# ======================================================== #

# Use this cloudbuild.yml to find the current hash of cached images:
# steps:
# - name: 'gcr.io/cloud-builders/docker'
#   args: ['image', 'ls', '--digests']


ENV DEBIAN_FRONTEND=noninteractive

ARG ACTION_RUNNER_VERSION=2.326.0
ARG GH_VERSION=2.75.1
ARG GH_PACKAGE=gh_${GH_VERSION}_linux_amd64.tar.gz
ARG GH_INSTALL_DIR=/opt/gh-cli-${GH_VERSION}

WORKDIR /actions-runner

# Install misc deps.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      adduser \
      ca-certificates \
      coreutils \
      curl \
      file \
      git \
      git-lfs \
      gosu \
      gzip \
      jq \
      lsb-release \
      shellcheck \
      sudo \
      tar \
      unzip \
      zip \
      zstd \
    && apt-get clean \
    && rm -r /var/lib/apt/lists/*

# Add the official Docker-in-Docker entrypoint script for a reliable DinD setup
COPY --from=docker:dind /usr/local/bin/dockerd-entrypoint.sh /usr/local/bin/

RUN curl -f -L -o runner.tar.gz https://github.com/actions/runner/releases/download/v${ACTION_RUNNER_VERSION}/actions-runner-linux-x64-${ACTION_RUNNER_VERSION}.tar.gz \
    && tar xzf ./runner.tar.gz \
    && rm runner.tar.gz \
    && rm -rf /var/lib/apt/lists/*

RUN /actions-runner/bin/installdependencies.sh \
    && adduser --disabled-password --gecos "" --uid 1001 runner \
    && usermod -aG sudo runner \
    # The 'docker-ce' package in the base image creates the 'docker' group automatically.
    # We just need to add our 'runner' user to it.
    && usermod -aG docker runner \
    # Configure sudoers securely \
    && mkdir -p /etc/sudoers.d/ \
    && echo "%sudo ALL=(ALL:ALL) NOPASSWD:ALL" > /etc/sudoers.d/runner_sudo_access \
    && echo "Defaults env_keep += \"DEBIAN_FRONTEND\"" >> /etc/sudoers.d/runner_sudo_access \
    # Define secure_path for sudo
    && echo "Defaults    secure_path = /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" >> /etc/sudoers.d/secure_path_default \
    && chmod 0440 /etc/sudoers.d/runner_sudo_access \
    && chmod 0440 /etc/sudoers.d/secure_path_default \
    && rm -rf /var/lib/apt/lists/*

# TODO: Install node separtely once image streaming is implemented.
# See https://github.com/abcxyz/github-action-dispatcher/issues/71
ENV PATH="/actions-runner/externals/node20/bin:$PATH"

# Install Python.
RUN apt-get update -y && \
  apt-get -y install --no-install-recommends \
  python3 \
  python3-dev \
  python3-pip \
  python3-venv \
  pipx \
  && rm -rf /var/lib/apt/lists/* \
  && python3 -m pipx ensurepath \
  && python3 --version

# Install gh CLI.
WORKDIR /install/github-cli/
RUN apt-get update && \
    apt-get install -y --no-install-recommends wget tar ca-certificates && \
    curl -f -L -o /tmp/${GH_PACKAGE} https://github.com/cli/cli/releases/download/v${GH_VERSION}/${GH_PACKAGE} -O  && \
    tar -xzf /tmp/${GH_PACKAGE} -C /opt && \
    mv /opt/gh_${GH_VERSION}_linux_amd64/bin/gh /usr/local/bin/gh && \
    chmod +x /usr/local/bin/gh && \
    gh --version && \
    rm -rf /tmp/${GH_PACKAGE} /opt/gh_${GH_VERSION}_linux_amd64 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Rename the real docker binary
RUN mv /usr/bin/docker /usr/bin/docker.real
# Add the wrapper script that will replace it
COPY docker_proxy.sh /usr/bin/docker
RUN chmod +x /usr/bin/docker

COPY start_runner.sh /actions-runner/start_runner.sh
RUN chmod +x /actions-runner/start_runner.sh

COPY setup_docker.sh /usr/local/bin/setup_docker.sh
RUN chmod +x /usr/local/bin/setup_docker.sh

# Mirror permissions from GitHub hosted runner. Note that in experimentation
# it appears that facl has been set, but I don't see that being done in this
# repo.
# https://github.com/actions/runner-images/blob/1ed26a6d42b1c856759a31823c9d99b9775cb5fa/images/ubuntu/scripts/build/configure-system.sh#L15
RUN chmod -R 777 /opt && \
    chmod -R 777 /usr/share

WORKDIR /home/runner

# We need to do some setup as the runner
USER runner
ENV HOME="/home/runner"
# .profile doesn't get sourced, so we need to add these manually
# This is a bit hacky as root user will also have /home/runner in its path.
ENV PATH="$HOME/bin:$HOME/.local/bin:$PATH"
# This exists in the runner, added for compatibility as setup-dotnet is not
# adding to path, breaking tools installed with `dotnet tool install`.
# TODO: remove when https://github.com/actions/setup-dotnet/issues/653 is
# resolved.
ENV PATH="$HOME/.dotnet/tools:$PATH"

# We may want to change to install globally, I think that is being done here
# by setting the PIPX_BIN_DIR to something in the path.
# https://github.com/actions/runner-images/blob/8701ae48e2da7e0b758c8e5982185514b915f52b/images/ubuntu/scripts/build/install-pipx-packages.sh#L4
RUN pipx ensurepath && \
    pipx install yamllint


USER root

# Docker socket is mounted at runtime, must fix permissions then.
ENTRYPOINT ["/usr/local/bin/setup_docker.sh"]
