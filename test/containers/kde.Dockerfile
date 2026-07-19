# Tier 2 (headless, real desktop secret stack) test environment: KDE's
# Secret Service daemon (ksecretd) and kwalletd6, unlocked non-interactively
# via pam-kwallet the same way a real login does — no display server
# needed. Fedora is used here instead of the tier-1 Debian image because
# Debian does not currently package ksecretd. Go is fetched at the "stable"
# release, matching the go-version used by the other CI jobs
# (actions/setup-go), rather than hand-pinned here.
FROM fedora:44

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN dnf install -y --setopt=install_weak_deps=False \
        kf6-kwallet pam-kwallet pamtester dbus-daemon socat util-linux \
        ca-certificates gcc make glibc-devel openssh-clients keyutils \
    && dnf clean all \
    && GO_VERSION=$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1) \
    && curl -fsSL "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY test/containers/kde-entrypoint.sh test/containers/kde-session.sh test/containers/kde.env test/containers/kde-pam.conf test/containers/kde-kwalletrc /opt/sshakku-tier2/
RUN chmod +x /opt/sshakku-tier2/kde-entrypoint.sh /opt/sshakku-tier2/kde-session.sh

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-tier2/kde-entrypoint.sh"]
