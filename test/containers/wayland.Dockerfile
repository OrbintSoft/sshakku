# Headless graphical session test environment: Wayland. The other images in
# this directory put their desktop on an X server; this one has no X server at
# all — sway drives wlroots' headless backend, so there is a screen with no DRM
# device, no seat and no privileges behind it.
#
# What is deliberately absent matters as much as what is here: no Xwayland, no
# Xvfb, no xdotool. A dialog that can only draw on X must fail here, which is
# the one thing an X11 image can never show. Windows are read back with
# swaymsg and typed into with wtype, which speaks Wayland's virtual-keyboard
# protocol; xdotool's counterpart needs an X server, and ydotool needs
# /dev/uinput and a privileged container.
#
# kdialog is absent too, and on purpose: a desktop that provides one dialog and
# not another is the ordinary case, not a special one. Go is fetched at the
# "stable" release, matching the go-version used by the other CI jobs
# (actions/setup-go), rather than hand-pinned here.
FROM debian:trixie-slim

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates wget gcc libc6-dev make openssh-client keyutils \
        dbus-daemon sway wtype jq zenity pinentry-gnome3 \
    && rm -rf /var/lib/apt/lists/* \
    && GO_VERSION=$(wget -qO- 'https://go.dev/VERSION?m=text' | head -n1) \
    && wget -qO- "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"

COPY test/containers/wayland-entrypoint.sh test/containers/wayland-session.sh /opt/sshakku-wayland/
COPY test/containers/wayland-compositor.sh test/containers/wayland-sway.config /opt/sshakku-wayland/
RUN chmod +x /opt/sshakku-wayland/wayland-entrypoint.sh /opt/sshakku-wayland/wayland-session.sh

WORKDIR /src

ENTRYPOINT ["/opt/sshakku-wayland/wayland-entrypoint.sh"]
