#!/bin/bash
# Sourced by a session script, not run on its own: makes the compartment the
# wallet keeps SSHakku's passphrases in, by running the command SSHakku offers
# a user for exactly that — `sshakku doctor --fix`. So the state every run in
# this image starts from is reached the way the user reaches it, rather than by
# a road only the test suite has.
#
# What making one takes is the wallet's decision, and GNOME Keyring's answer is
# a dialog; the caller supplies the part that differs between sessions — how a
# button is pressed — around these two functions.
#
# The compartment is given a blank password, which is also what makes every
# later unlock of it prompt-free rather than only this one creation. Unlike
# KDE's kwalletrc, gnome-keyring has no config file that lets an unattended
# session pre-seed a collection.
#
# Must be used from the module root (go.mod), with D-Bus and
# gnome-keyring-daemon already up.
# shellcheck shell=bash

# The compartment SSHakku makes for itself when the configuration names none,
# which is the one the runs that follow expect to find.
compartment_name="sshakku"
compartment_binary="/tmp/sshakku-make-compartment"
compartment_log="/tmp/make-compartment.log"

# How long the whole attempt may take. A dialog nobody answers is waited on for
# as long as it is up, so without a bound a missed press would hang the session
# instead of failing it.
compartment_budget=180

# compartment_collection_paths and compartment_label ask the wallet itself, over
# the bus: whether the setup produced the state it exists to produce is judged
# from outside the thing that produced it, not by asking SSHakku again.
compartment_collection_paths() {
	dbus-send --session --print-reply --dest=org.freedesktop.secrets \
		/org/freedesktop/secrets org.freedesktop.DBus.Properties.Get \
		string:org.freedesktop.Secret.Service string:Collections 2>/dev/null |
		sed -n 's/.*object path "\([^"]*\)".*/\1/p'
}

compartment_label() {
	dbus-send --session --print-reply --dest=org.freedesktop.secrets \
		"$1" org.freedesktop.DBus.Properties.Get \
		string:org.freedesktop.Secret.Collection string:Label 2>/dev/null |
		sed -n 's/.*string "\([^"]*\)".*/\1/p'
}

compartment_exists() {
	local path
	while read -r path; do
		if [ "$(compartment_label "${path}")" = "${compartment_name}" ]; then
			return 0
		fi
	done < <(compartment_collection_paths)
	return 1
}

# start_making_the_compartment returns as soon as the binary is running, leaving
# it waiting on whatever the wallet raised. Its pid is published so the caller
# can stop answering the moment the run is over.
start_making_the_compartment() {
	go build -o "${compartment_binary}" ./cmd/sshakku
	timeout "${compartment_budget}" "${compartment_binary}" doctor --fix >"${compartment_log}" 2>&1 &
	compartment_pid=$!
}

# finish_making_the_compartment ends the session when the compartment is not
# there, rather than letting the run that follows discover it: a dialog can be
# missed instead of refused — a press lost to render timing, a window that never
# appeared — and a wallet with nowhere to store anything fails somewhere far
# from the cause.
finish_making_the_compartment() {
	local script="${BASH_SOURCE[-1]##*/}"

	if ! wait "${compartment_pid}"; then
		echo "${script}: sshakku doctor --fix did not come back having done its work:" >&2
		cat "${compartment_log}" >&2
		exit 1
	fi

	if ! compartment_exists; then
		echo "${script}: the wallet holds no ${compartment_name} compartment after --fix:" >&2
		cat "${compartment_log}" >&2
		exit 1
	fi
}
