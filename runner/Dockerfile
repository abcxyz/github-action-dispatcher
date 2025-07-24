FROM gcr.io/cloud-builders/docker@sha256:057d91e53efd3e025350d09ecf0fea30ec261bdc5ef5e052d1351c6c6d9dfe21
# Use this to find the current hash of cached images.
# docker image ls --digests
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

WORKDIR /home/runner

# We need to do some setup as the runner
USER runner
ENV HOME="/home/runner"
# .profile doesn't get sourced, so we need to add these manually
# This is a bit hacky as root user will also have /home/runner in its path.
ENV PATH="$HOME/bin:$HOME/.local/bin:$PATH"

# We may want to change to install globally, I think that is being done here
# by setting the PIPX_BIN_DIR to something in the path.
# https://github.com/actions/runner-images/blob/8701ae48e2da7e0b758c8e5982185514b915f52b/images/ubuntu/scripts/build/install-pipx-packages.sh#L4
RUN pipx ensurepath && \
    pipx install yamllint


USER root

# Docker socket is mounted at runtime, must fix permissions then.
ENTRYPOINT ["/usr/local/bin/setup_docker.sh"]
