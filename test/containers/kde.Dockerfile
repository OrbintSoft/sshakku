# Headless, real desktop secret stack test environment: KDE's
# Secret Service daemon (ksecretd) and kwalletd6, unlocked non-interactively
# via pam-kwallet the same way a real login does. Fedora is used here instead
# of the container test suite's Debian image because Debian does not currently
# package ksecretd. Go is fetched at the "stable" release, matching the
# go-version used by the other CI jobs (actions/setup-go), rather than
# hand-pinned here.
#
# Three sessions are reachable in this one image, since what differs between
# them is the login and not the wallet: by default there is no display server at
# all, SSHAKKU_SESSION_SCRIPT=kde-wayland-session.sh puts a sway compositor
# around the same unlock, and kde-x11-session.sh an Xvfb display. Which one Qt
# then draws on it works out for itself, so both plugins are here:
# qt6-qtwayland for the compositor (sway drives wlroots' headless backend, with
# no X server behind it) and qt6-qtbase-gui's xcb for the X server.
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
        sway qt6-qtwayland xorg-x11-server-Xvfb \
        ca-certificates gcc make glibc-devel openssh-clients keyutils \
    && dnf clean all \
    && setcap -r /usr/sbin/sway \
    && GO_VERSION=$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1) \
    && curl -fsSL "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY test/containers/kde-entrypoint.sh test/containers/kde-session.sh test/containers/kde-wayland-session.sh test/containers/kde-x11-session.sh test/containers/kde.env test/containers/kde-pam.conf test/containers/kde-kwalletrc /opt/sshakku-desktop-stack/
COPY test/containers/wayland-compositor.sh test/containers/wayland-sway.config /opt/sshakku-desktop-stack/
RUN chmod +x /opt/sshakku-desktop-stack/kde-entrypoint.sh /opt/sshakku-desktop-stack/kde-session.sh /opt/sshakku-desktop-stack/kde-wayland-session.sh /opt/sshakku-desktop-stack/kde-x11-session.sh

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-desktop-stack/kde-entrypoint.sh"]
