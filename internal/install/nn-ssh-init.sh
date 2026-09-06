#!/bin/bash

# SSH bootstrap, sourced from a login shell's startup files (system-wide or
# per-user; bash or zsh — this script uses no bash-only syntax). A non-login
# shell's startup files may also reach it indirectly, via a thin `source`
# wrapper pointing back at this same installed copy.
#
# This is a thin hook around the sshakku core. `sshakku shell-init`, evaluated
# below, keeps an ssh-agent healthy on a fixed socket and prints the runtime
# paths to use; the fixed socket means the SSH_AUTH_SOCK we export never goes
# stale even if the agent is restarted. `sshakku askpass-env` then routes this
# shell's ssh passphrase prompts through sshakku's wallet-aware askpass broker.
# In interactive shells `sshakku load-keys` also adds the user's keys, pulling
# each passphrase from the OS secret store and skipping any key already in the
# agent. All the logic lives in the core; this script only pins the shell to
# the socket and invokes it.

sshakku_bin="/usr/local/bin/sshakku"

# Resolve the runtime paths, keep the agent healthy, and print the shell
# assignments to eval. Declare them first so an absent or failing binary leaves
# them empty rather than unset.
agent_sock=""
log_file=""
ssh_tools_dir=""
if [ -x "$sshakku_bin" ]; then
	eval "$("$sshakku_bin" shell-init)"
fi
# Without the resolved paths there is nothing we can safely do.
[ -n "$agent_sock" ] && [ -n "$log_file" ] || return

# Always pin this shell -- and, at login, the whole session -- to the fixed path.
export SSH_AUTH_SOCK="$agent_sock"
unset SSH_AGENT_PID

# The endpoint above is only worth having if the ssh this shell runs can open
# it. Where the first ssh on this PATH cannot, sshakku names a directory holding
# one that can, already spelled the way this shell reads a path, and it goes in
# front so that every ssh started here -- typed, or started by git -- reaches
# the agent the shell was just pointed at. It names nothing where the ssh
# already first can reach it, which is every system with one OpenSSH on it.
if [ -n "$ssh_tools_dir" ]; then
	PATH="$ssh_tools_dir:$PATH"
	export PATH
fi

# Wired in every login shell, not just interactive ones: some environments
# resolve a terminal's inherited environment via a non-interactive login shell,
# so gating this on interactivity would silently drop it there. It only prints
# export lines, so it stays cheap for non-interactive logins too.
eval "$("$sshakku_bin" askpass-env)"

# Load keys only in interactive shells: key loading may prompt and writes to the
# terminal, which must never happen for non-interactive sessions (scp/rsync/git).
if [[ $- == *i* ]]; then
	"$sshakku_bin" load-keys
fi
