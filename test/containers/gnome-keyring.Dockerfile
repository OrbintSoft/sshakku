# Headless, real desktop secret stack test environment: GNOME
# Keyring's Secret Service daemon (gnome-keyring-daemon). Unlike KDE's
# ksecretd, gnome-keyring only auto-unlocks non-interactively via PAM for
# its single hardcoded "login" collection; a distinctly named collection
# ("sshakku") always requires one interactive creation dialog, so this image
# drives that one-time dialog headlessly. Go is fetched at the "stable"
# release, matching the go-version used by the other CI jobs
# (actions/setup-go), rather than hand-pinned here.
#
# Two sessions are reachable in this one image, since the wallet is the same in
# both and only the login differs: an X server by default, where the dialog is
# clicked with xdotool, and a sway compositor when SSHAKKU_SESSION_SCRIPT names
# that session, where it is clicked through the compositor's own cursor. The
# second needs a pointer device to exist at all — see wayland-pointer.sh — which
# is what seatd and the uinput-pointer tool below are for.
FROM debian:trixie-slim

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates wget gcc libc6-dev make openssh-client keyutils \
        gnome-keyring libsecret-tools dbus-x11 xvfb xdotool \
        sway seatd jq \
    && rm -rf /var/lib/apt/lists/* \
    && GO_VERSION=$(wget -qO- 'https://go.dev/VERSION?m=text' | head -n1) \
    && wget -qO- "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY test/containers/gnome-keyring-entrypoint.sh test/containers/gnome-keyring-session.sh test/containers/gnome-keyring-create-collection.sh /opt/sshakku-desktop-stack/
COPY test/containers/gnome-keyring-start.sh test/containers/gnome-keyring-wayland-session.sh test/containers/gnome-keyring-wayland-create-collection.sh /opt/sshakku-desktop-stack/
COPY test/containers/wayland-compositor.sh test/containers/wayland-pointer.sh test/containers/wayland-sway.config /opt/sshakku-desktop-stack/
RUN chmod +x /opt/sshakku-desktop-stack/gnome-keyring-entrypoint.sh /opt/sshakku-desktop-stack/gnome-keyring-session.sh /opt/sshakku-desktop-stack/gnome-keyring-create-collection.sh \
    /opt/sshakku-desktop-stack/gnome-keyring-wayland-session.sh /opt/sshakku-desktop-stack/gnome-keyring-wayland-create-collection.sh

# The pointer tool is a program of this environment, not of SSHakku: it carries
# a build tag that keeps it out of the module and is compiled on its own here.
COPY test/containers/uinput_pointer_linux.go /opt/sshakku-desktop-stack/
RUN go build -o /usr/local/bin/uinput-pointer /opt/sshakku-desktop-stack/uinput_pointer_linux.go

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-desktop-stack/gnome-keyring-entrypoint.sh"]
