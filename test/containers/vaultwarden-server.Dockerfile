# The fixture server, built around the certificate generated for the run that
# builds it — see vaultwarden-server.sh, which assembles the context.
#
# The database and the certificate are baked in rather than mounted from the
# host, because the host may not be able to offer a mount at all: on macOS the
# container runtime lives in a virtual machine that sees only part of the
# filesystem, and a bind mount of a directory it cannot see silently becomes an
# empty one. Baking them in also keeps the database on the container's own
# filesystem, where SQLite's locking behaves, and keeps everything the server
# writes off the host entirely.
#
# The version is pinned so a run's outcome cannot change under it, and it has to
# be kept recent: the bw CLI is a client of a server API, and one too old for the
# CLI the tests drive fails at creating an item — client-side, inside bw's own
# SDK, which reads as a broken client rather than as a mismatched pair.
# Vaultwarden is an
# AGPL-3.0 reimplementation of the Bitwarden server API, used transiently inside
# a disposable test container, never modified and never offered as a service, so
# the network-copyleft clause does not apply.
FROM vaultwarden/server:1.37.1

COPY db.sqlite3 rsa_key.pem /data/
COPY cert.pem key.pem /ssl/
