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
# The version is pinned because the committed fixture database was produced
# against it and is only guaranteed to be readable by it. Vaultwarden is an
# AGPL-3.0 reimplementation of the Bitwarden server API, used transiently inside
# a disposable test container, never modified and never offered as a service, so
# the network-copyleft clause does not apply.
FROM vaultwarden/server:1.36.0

COPY db.sqlite3 rsa_key.pem /data/
COPY cert.pem key.pem /ssl/
