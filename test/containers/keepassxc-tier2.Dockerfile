# Tier 2 (headless, real desktop secret stack) test environment: KeePassXC's
# Secret Service D-Bus integration. Unlike ksecretd/gnome-keyring-daemon,
# KeePassXC has no standalone daemon mode — a "collection" is an open
# database tab inside the full GUI app, so this image drives the one-time
# "create new database" wizard headlessly via Xvfb + xdotool, the same shape
# already used for the GNOME Keyring row. Fedora is used instead of the
# tier-1 Debian image because Debian trixie's keepassxc package (2.7.10)
# segfaults whenever it unlocks a database while running as a backgrounded
# process, which Fedora's newer build (2.7.12) does not. Go is fetched at
# the "stable" release, matching the go-version used by the other CI jobs
# (actions/setup-go), rather than hand-pinned here.
FROM fedora:44

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN dnf install -y --setopt=install_weak_deps=False \
        keepassxc dbus-daemon xorg-x11-server-Xvfb xdotool util-linux procps-ng \
        ca-certificates gcc make glibc-devel openssh-clients keyutils \
    && dnf clean all \
    && GO_VERSION=$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1) \
    && curl -fsSL "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY test/containers/keepassxc-tier2-entrypoint.sh test/containers/keepassxc-tier2-session.sh test/containers/keepassxc-tier2-create-collection.sh /opt/sshakku-tier2/
RUN chmod +x /opt/sshakku-tier2/keepassxc-tier2-entrypoint.sh /opt/sshakku-tier2/keepassxc-tier2-session.sh /opt/sshakku-tier2/keepassxc-tier2-create-collection.sh

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-tier2/keepassxc-tier2-entrypoint.sh"]
