####################
# Version pins: mise.toml at the repo root is the source of truth.
# The ARG defaults below are generated from it by `mise run pins:sync`
# (verified in CI by `pins:check`); `mise run docker:build` also injects
# them explicitly via --build-arg.
####################
# Base image with common dependencies and Japanese fonts
FROM ubuntu:26.04 AS base

ARG DEBIAN_FRONTEND=noninteractive
ARG TZ=Asia/Tokyo
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update && apt-get install -y --no-install-recommends \
  # Common tools
  curl unzip wget ca-certificates git build-essential pkg-config \
  # Japanese fonts and minimal locale support
  fonts-noto-cjk fonts-noto-cjk-extra locales \
  && update-ca-certificates \
  # Generate only required locales (ja_JP, en_US) to reduce image size
  && printf 'ja_JP.UTF-8 UTF-8\n' > /etc/locale.gen \
  && printf 'en_US.UTF-8 UTF-8\n' >> /etc/locale.gen \
  && locale-gen \
  && update-locale LANG=ja_JP.UTF-8 \
  # TimeZone settings
  && ln -snf /usr/share/zoneinfo/"$TZ" /etc/localtime \
  && echo "$TZ" > /etc/timezone \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8
ENV LC_CTYPE=ja_JP.UTF-8

####################
# Stage 1: Build Neovim from source
FROM base AS nvim-builder

ARG NEOVIM_VERSION=0.12.4
RUN apt-get update && apt-get install -y --no-install-recommends \
  ninja-build gettext cmake \
  && git clone https://github.com/neovim/neovim /neovim \
  && cd /neovim \
  && git checkout "v$NEOVIM_VERSION" \
  && make -j"$(nproc)" CMAKE_BUILD_TYPE=RelWithDebInfo \
  && make install DESTDIR=/neovim-install \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

####################
# Stage 2: Build Node runtime and npm tools
FROM base AS node-builder

ARG NODE_VERSION=24.18.0
ARG NPM_VERSION=11.18.0
ENV NODE_HOME="/opt/node"
ENV PATH="${NODE_HOME}/bin:${PATH}"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN apt-get update && apt-get install -y --no-install-recommends xz-utils \
  && ARCH="$(dpkg --print-architecture)" \
  && case "$ARCH" in \
  amd64) NODE_ARCH="linux-x64" ;; \
  arm64) NODE_ARCH="linux-arm64" ;; \
  *) echo "Unsupported arch: $ARCH" && exit 1 ;; \
  esac \
  && cd /tmp \
  && NODE_TARBALL="node-v${NODE_VERSION}-${NODE_ARCH}.tar.xz" \
  && NODE_URL="https://nodejs.org/dist/v${NODE_VERSION}/${NODE_TARBALL}" \
  && curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$NODE_URL" -o "$NODE_TARBALL" \
  && SHA_URL="https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" \
  && curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$SHA_URL" -o SHASUMS256.txt \
  && grep " ${NODE_TARBALL}$" SHASUMS256.txt | sha256sum -c - \
  && tar -xJf "$NODE_TARBALL" \
  && mkdir -p "$NODE_HOME" \
  && mv "/tmp/node-v${NODE_VERSION}-${NODE_ARCH}"/* "$NODE_HOME"/ \
  && rm -rf "/tmp/node-v${NODE_VERSION}-${NODE_ARCH}" "$NODE_TARBALL" SHASUMS256.txt \
  && npm install -g npm@"$NPM_VERSION" \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

COPY ./environment/tools/node/ /opt/npm-tools/
RUN --mount=type=cache,target=/root/.npm,sharing=locked \
  cd /opt/npm-tools && npm install --omit=dev --no-audit --no-fund

####################
# Stage 3: Build Go toolchain and tools
FROM base AS go-builder

ARG GO_VERSION=1.26.4

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN ARCH="$(dpkg --print-architecture)" \
  && case "$ARCH" in \
  amd64) GO_ARCH="linux-amd64" ;; \
  arm64) GO_ARCH="linux-arm64" ;; \
  *) echo "Unsupported arch: $ARCH" && exit 1 ;; \
  esac \
  && GO_TARBALL="go${GO_VERSION}.${GO_ARCH}.tar.gz" \
  && GO_BASE_URL="https://dl.google.com/go" \
  && curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "${GO_BASE_URL}/${GO_TARBALL}" -o "$GO_TARBALL" \
  && curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "${GO_BASE_URL}/${GO_TARBALL}.sha256" -o "${GO_TARBALL}.sha256" \
  && GO_HASH_REF="$(tr -d ' \r\n' < "${GO_TARBALL}.sha256")" \
  && GO_HASH_ACTUAL="$(sha256sum "$GO_TARBALL" | awk '{print $1}')" \
  && [ "$GO_HASH_ACTUAL" = "$GO_HASH_REF" ] || (echo "Go tarball checksum mismatch: expected=$GO_HASH_REF actual=$GO_HASH_ACTUAL" >&2; exit 1) \
  && tar -C /usr/local -xzf "$GO_TARBALL" \
  && rm "$GO_TARBALL" "${GO_TARBALL}.sha256"

ENV PATH="/usr/local/go/bin:${PATH}"
COPY ./environment/tools/go/go-tools.txt /tmp/go-tools.txt
RUN --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    --mount=type=cache,target=/root/go/pkg/mod,sharing=locked \
    while read -r pkg; do \
      CGO_ENABLED=0 go install "$pkg" || exit 1; \
    done < /tmp/go-tools.txt

####################
# Stage 4: Build Rust-based tools
FROM base AS rust-builder

ARG RUST_TOOLCHAIN=stable
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN apt-get update && apt-get install -y --no-install-recommends \
  clang libclang-dev \
  && curl --proto '=https' --tlsv1.2 --retry 5 --retry-all-errors --retry-delay 2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain "$RUST_TOOLCHAIN" \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*
ENV PATH="/root/.cargo/bin:${PATH}"
RUN --mount=type=cache,target=/root/.cargo/registry,sharing=locked \
    --mount=type=cache,target=/root/.cargo/git,sharing=locked \
    cargo install stylua tree-sitter-cli

####################
# Stage 5: Build Python venv with uv
FROM base AS python-builder

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN apt-get update && apt-get install -y --no-install-recommends python3 python3-venv \
  && curl --retry 5 --retry-all-errors --retry-delay 2 -LsSf https://astral.sh/uv/install.sh | sh \
  && export PATH="$HOME/.local/bin:$PATH" \
  && uv --version >/dev/null \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

COPY ./environment/tools/python/pyproject.toml /opt/python/
WORKDIR /opt/python
RUN export PATH="$HOME/.local/bin:$PATH" \
  && uv sync --project .
WORKDIR /

####################
# Stage 6: Fetch hadolint binary only
FROM base AS hadolint-builder

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN set -eux; \
  ARCH="$(dpkg --print-architecture)"; \
  case "$ARCH" in \
    amd64) HL_ARCH="Linux-x86_64" ;; \
    arm64) HL_ARCH="Linux-arm64" ;; \
    *) echo "Unsupported arch for hadolint: $ARCH" >&2; exit 1 ;; \
  esac; \
  BASE_URL="https://github.com/hadolint/hadolint/releases/latest/download"; \
  TMPDIR="/tmp/hadolint"; \
  mkdir -p "$TMPDIR"; \
  BIN_PATH="$TMPDIR/hadolint"; \
  SHA_PATH="$TMPDIR/hadolint.sha256"; \
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$BASE_URL/hadolint-$HL_ARCH" -o "$BIN_PATH"; \
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$BASE_URL/hadolint-$HL_ARCH.sha256" -o "$SHA_PATH"; \
  REF_SHA="$(awk '{print $1}' "$SHA_PATH")"; \
  ACT_SHA="$(sha256sum "$BIN_PATH" | awk '{print $1}')"; \
  [ "$ACT_SHA" = "$REF_SHA" ] || (echo "hadolint checksum mismatch: expected=$REF_SHA actual=$ACT_SHA" >&2; exit 1); \
  install -m 0755 "$BIN_PATH" /usr/local/bin/hadolint

####################
# Stage 7: Fetch Terraform CLI binary only
FROM base AS terraform-builder

ARG TERRAFORM_VERSION=1.15.7
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN set -eux; \
  ARCH="$(dpkg --print-architecture)"; \
  case "$ARCH" in \
    amd64) TF_ARCH="linux_amd64" ;; \
    arm64) TF_ARCH="linux_arm64" ;; \
    *) echo "Unsupported arch for terraform: $ARCH" >&2; exit 1 ;; \
  esac; \
  BASE_URL="https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}"; \
  TMPDIR="/tmp/terraform"; \
  mkdir -p "$TMPDIR"; \
  ZIP_NAME="terraform_${TERRAFORM_VERSION}_${TF_ARCH}.zip"; \
  SUMS_NAME="terraform_${TERRAFORM_VERSION}_SHA256SUMS"; \
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$BASE_URL/$ZIP_NAME" -o "$TMPDIR/$ZIP_NAME"; \
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$BASE_URL/$SUMS_NAME" -o "$TMPDIR/$SUMS_NAME"; \
  (cd "$TMPDIR" && grep " ${ZIP_NAME}$" "$SUMS_NAME" | sha256sum -c -); \
  unzip -o "$TMPDIR/$ZIP_NAME" -d "$TMPDIR"; \
  install -m 0755 "$TMPDIR/terraform" /usr/local/bin/terraform

####################
# Final stage
FROM base

COPY ./nvim/config/.bash_profile /root/

RUN apt-get update && apt-get install -y --no-install-recommends \
  ripgrep fd-find python3 mysql-client luarocks \
  && mkdir -p /root/.local/state/nvim/undo \
  && apt-get autoremove -y \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

ENV PYTHONIOENCODING=utf-8

####################
# Bring toolchains and Neovim from builders
COPY --from=go-builder /usr/local/go/ /usr/local/go/
COPY --from=go-builder /root/go/bin/ /root/go/bin/
COPY --from=hadolint-builder /usr/local/bin/hadolint /usr/local/bin/hadolint
COPY --from=terraform-builder /usr/local/bin/terraform /usr/local/bin/terraform
COPY --from=nvim-builder /neovim-install/ /
COPY --from=node-builder /opt/node/ /opt/node/
COPY --from=node-builder /opt/npm-tools/ /opt/npm-tools/
COPY --from=python-builder /opt/python/.venv/ /opt/python/.venv/
COPY --from=python-builder /root/.local/bin/uv /usr/local/bin/uv
COPY --from=python-builder /root/.local/bin/uvx /usr/local/bin/uvx
COPY --from=rust-builder /root/.cargo/bin/stylua /usr/local/bin/stylua
COPY --from=rust-builder /root/.cargo/bin/tree-sitter /usr/local/bin/tree-sitter

ENV PATH="/opt/python/.venv/bin:/opt/node/bin:/opt/npm-tools/node_modules/.bin:/usr/local/go/bin:/root/go/bin:${PATH}"
ENV NODE_PATH="/opt/npm-tools/node_modules"

####################
# Copy Neovim configs after build for better caching
COPY ./nvim/config/init.lua /root/.config/nvim/
COPY ./nvim/config/lua/ /root/.config/nvim/lua/
COPY ./environment/tools/python/ruff.toml /root/.config/ruff/

WORKDIR /workspace

HEALTHCHECK --interval=10m --timeout=1m --start-period=10m --retries=1 \
  CMD nvim --headless -c 'checkhealth' -c 'qall' 2>/dev/null || exit 1
