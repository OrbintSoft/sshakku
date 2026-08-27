# A Windows machine with KeePassXC on it and nothing else — the machine of
# somebody who keeps their passwords in KeePassXC and has just met SSHakku.
#
# It is a second image rather than a line added to windows-servercore.Dockerfile
# because that one's premise is that nothing is installed: a scenario that asks
# what SSHakku does on a machine that has never been set up cannot be run on a
# machine carrying a wallet. Which programs are present is the thing under test
# in both, so the two cannot share one image.
#
# Only KeePassXC's command-line tool is put here, and that is the whole of what
# the route under test needs: it opens the database file itself and talks to no
# running KeePassXC. The graphical program is not installed and could not be
# driven here anyway — Server Core has no desktop to draw it on, which is why
# the local-protocol route is covered elsewhere and not by this image.
#
# Only the base image's own build can be run under process isolation, and only
# on a host of exactly that build; anywhere else the caller asks for Hyper-V
# isolation, which does not care. The runner script takes that as an argument.
FROM mcr.microsoft.com/windows/servercore:ltsc2025

# KeePassXC first, so the layer that fetches 30-odd megabytes over the network
# is not rebuilt every time the program under test is.
# KeePassXC and the Microsoft C++ runtime it links against, both put into the
# build context by windows-keepassxc-prepare.ps1 before this is built. The
# runtime is not optional here: KeePassXC's archive ships none, a desktop has it
# installed already and this image does not, and a program that cannot resolve
# its imports dies before main with nothing on either stream. Copied rather than installed by a RUN step, and that is
# a constraint rather than a preference: a RUN here executes under process
# isolation, which needs the host's Windows build to be exactly this image's,
# and a host whose build has moved on cannot start such a container at all.
# Every Windows image in this directory is COPY and nothing else for that
# reason — anything one of them needs is fetched outside and handed in.
COPY KeePassXC C:/KeePassXC

# Putting it on PATH is the scenario's job, not this file's, and that is the
# same constraint again. A Windows base image declares no PATH of its own —
# what a process there searches comes from the machine's registry — so an ENV
# here does not add to that list, it replaces it, and the container then cannot
# find powershell to start with. Writing the list out in full instead would
# have this file restating something the base image owns. On a real machine an
# installer records the entry; here the scenario does, where it can be seen.

# The program under test, built for this platform by whoever runs the scenario.
# It is put somewhere no session searches, so a scenario that means to check
# what the install does to PATH cannot pass because the shell found it anyway.
COPY sshakku.exe C:/sshakku-under-test/sshakku.exe

# The askpass helper, which is the same program under a second name and is how a
# passphrase reaches ssh-add without passing through anybody's command line.
COPY sshakku.exe C:/sshakku-under-test/sshakku-askpass.exe

COPY *.ps1 C:/scenario/
COPY windows-keepassxc-config.toml C:/scenario/windows-keepassxc-config.toml

# No CMD: every scenario is named by the caller, so an image run with nothing
# said does nothing rather than something arbitrary.
