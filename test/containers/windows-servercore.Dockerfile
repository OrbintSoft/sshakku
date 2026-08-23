# A Windows machine on which nobody has installed anything — which is what a
# person meets the first time they run SSHakku, and what a machine that has been
# developed on can no longer be. A container is the only way to get one back:
# the wiring SSHakku writes lives in a user's profile, in that account's stored
# environment and in a service the whole machine shares, so a throwaway
# directory does not isolate it and a second account does not either.
#
# Nothing is installed here on purpose. This image ships the OpenSSH that
# SSHakku drives — ssh.exe, ssh-add.exe and the ssh-agent service, the latter
# present and disabled, as on a machine nobody has set up — so a scenario that
# passes has driven the same programs a user has, not a substitute chosen to
# make it pass.
#
# Only the base image's own build can be run under process isolation, and only
# on a host of exactly that build; anywhere else the caller asks for Hyper-V
# isolation, which does not care. The runner script takes that as an argument.
FROM mcr.microsoft.com/windows/servercore:ltsc2025

# The program under test, built for this platform by whoever runs the scenario.
# It is put somewhere no session searches, so a scenario that means to check
# what the install does to PATH cannot pass because the shell found it anyway.
COPY sshakku.exe C:/sshakku-under-test/sshakku.exe

# The askpass helper, which is the same program under a second name and is how a
# passphrase reaches ssh-add without passing through anybody's command line.
# Putting it here is what `make install` does when it puts the program in place;
# a scenario would otherwise be testing a half-installed program, and the way it
# fails is silent — ssh-add is pointed at a helper that is not there, answers
# nothing, and the session log reports a stored passphrase as having gone stale.
COPY sshakku.exe C:/sshakku-under-test/sshakku-askpass.exe

COPY *.ps1 C:/scenario/

# No CMD: every scenario is named by the caller, so an image run with nothing
# said does nothing rather than something arbitrary.
