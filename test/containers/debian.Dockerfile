# Headless, no-desktop test environment: a widely-used systemd
# distro. Go is fetched at the "stable" release, matching the go-version
# used by the other CI jobs (actions/setup-go), rather than hand-pinned
# here.
FROM debian:bookworm-slim

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates wget gcc libc6-dev make openssh-client keyutils bats \
    && rm -rf /var/lib/apt/lists/* \
    && GO_VERSION=$(wget -qO- 'https://go.dev/VERSION?m=text' | head -n1) \
    && wget -qO- "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /src
