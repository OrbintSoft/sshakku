# Tier 2 (headless, real desktop secret stack) test environment: GNOME
# Keyring's Secret Service daemon (gnome-keyring-daemon). Unlike KDE's
# ksecretd, gnome-keyring only auto-unlocks non-interactively via PAM for
# its single hardcoded "login" collection; a distinctly named collection
# ("sshakku") always requires one interactive creation dialog, so this image
# drives that one-time dialog headlessly via Xvfb + xdotool instead. Go is
# fetched at the "stable" release, matching the go-version used by the other
# CI jobs (actions/setup-go), rather than hand-pinned here.
FROM debian:trixie-slim

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates wget gcc libc6-dev make openssh-client keyutils \
        gnome-keyring libsecret-tools dbus-x11 xvfb xdotool \
    && rm -rf /var/lib/apt/lists/* \
    && GO_VERSION=$(wget -qO- 'https://go.dev/VERSION?m=text' | head -n1) \
    && wget -qO- "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY test/containers/gnome-keyring-entrypoint.sh test/containers/gnome-keyring-session.sh test/containers/gnome-keyring-create-collection.sh /opt/sshakku-tier2/
RUN chmod +x /opt/sshakku-tier2/gnome-keyring-entrypoint.sh /opt/sshakku-tier2/gnome-keyring-session.sh /opt/sshakku-tier2/gnome-keyring-create-collection.sh

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-tier2/gnome-keyring-entrypoint.sh"]
