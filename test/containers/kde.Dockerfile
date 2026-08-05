# Headless, real desktop secret stack test environment: KDE's
# Secret Service daemon (ksecretd) and kwalletd6, unlocked non-interactively
# via pam-kwallet the same way a real login does. Fedora is used here instead
# of the container test suite's Debian image because Debian does not currently
# package ksecretd. Go is fetched at the "stable" release, matching the
# go-version used by the other CI jobs (actions/setup-go), rather than
# hand-pinned here.
#
# Two sessions are reachable in this one image, since what differs between them
# is the login and not the wallet: by default there is no display server at all,
# and SSHAKKU_SESSION_SCRIPT=kde-wayland-session.sh puts a sway compositor
# around the same unlock. There is no X server in either — sway drives wlroots'
# headless backend, and qt6-qtwayland is the plugin Qt needs to draw on it.
#
# This sway carries cap_sys_nice, which an ordinary container is not allowed to
# grant: the file capability is not in the default bounding set, so executing it
# fails outright with "Operation not permitted" before the compositor starts.
# The capability only buys realtime scheduling priority for a compositor driving
# real hardware, so it is stripped here rather than handed to the container.
FROM fedora:44

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN dnf install -y --setopt=install_weak_deps=False \
        kf6-kwallet pam-kwallet pamtester dbus-daemon socat util-linux \
        sway qt6-qtwayland \
        ca-certificates gcc make glibc-devel openssh-clients keyutils \
    && dnf clean all \
    && setcap -r /usr/sbin/sway \
    && GO_VERSION=$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1) \
    && curl -fsSL "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY test/containers/kde-entrypoint.sh test/containers/kde-session.sh test/containers/kde-wayland-session.sh test/containers/kde.env test/containers/kde-pam.conf test/containers/kde-kwalletrc /opt/sshakku-desktop-stack/
COPY test/containers/wayland-compositor.sh test/containers/wayland-sway.config /opt/sshakku-desktop-stack/
RUN chmod +x /opt/sshakku-desktop-stack/kde-entrypoint.sh /opt/sshakku-desktop-stack/kde-session.sh /opt/sshakku-desktop-stack/kde-wayland-session.sh

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-desktop-stack/kde-entrypoint.sh"]
