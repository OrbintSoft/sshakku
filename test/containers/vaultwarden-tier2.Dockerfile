# Tier 2 (headless, real secret-store backend) test environment: a
# self-hosted Vaultwarden server (an AGPL-3.0 reimplementation of the
# Bitwarden server API) plus the bw CLI. Unlike KDE/GNOME/KeePassXC, there is
# no desktop session or Xvfb here — Vaultwarden is a plain HTTP daemon, and
# `bw` talks to it directly. `bw` has no account-registration command (the
# client-side master-password key derivation and RSA keypair generation only
# exist in the web-vault UI), so this image ships a pre-registered,
# already-empty test account as a SQLite fixture
# (vaultwarden-tier2-fixture/) instead of registering one at container
# startup — see PLAN.md 4.2 for how that fixture was produced. The
# Vaultwarden binary is copied from the upstream image rather than built
# here, at the same pinned version the fixture was produced against; only
# used transiently inside this disposable CI container, never modified or
# offered as a service, so AGPL-3.0's network-copyleft clause does not apply
# (rule 16). Go is fetched at the "stable" release, matching the go-version
# used by the other CI jobs (actions/setup-go), rather than hand-pinned here.
FROM vaultwarden/server:1.36.0 AS vaultwarden

FROM debian:trixie-slim

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates wget gcc libc6-dev make openssh-client keyutils \
        nodejs npm openssl sqlite3 libmariadb3 libpq5 \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @bitwarden/cli@2026.6.0 \
    && GO_VERSION=$(wget -qO- 'https://go.dev/VERSION?m=text' | head -n1) \
    && wget -qO- "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY --from=vaultwarden /vaultwarden /usr/local/bin/vaultwarden

COPY test/containers/vaultwarden-tier2-fixture/ /opt/sshakku-tier2/vaultwarden-fixture/
COPY test/containers/vaultwarden-tier2-entrypoint.sh test/containers/vaultwarden-tier2-session.sh /opt/sshakku-tier2/
RUN chmod +x /opt/sshakku-tier2/vaultwarden-tier2-entrypoint.sh /opt/sshakku-tier2/vaultwarden-tier2-session.sh

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-tier2/vaultwarden-tier2-entrypoint.sh"]
