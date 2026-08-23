# SSH bootstrap, dot-sourced from a PowerShell profile — per-user or
# all-users, Windows PowerShell 5.x or PowerShell 7 and later. A profile that
# reads a Profile.d directory reaches this same installed copy through the
# small wrapper dropped in there.
#
# This is a thin hook around the sshakku core. `sshakku shell-init`, evaluated
# below, keeps an ssh-agent healthy on a fixed endpoint and prints the runtime
# paths to use; the fixed endpoint means the SSH_AUTH_SOCK this session gets
# never goes stale even if the agent is restarted. `sshakku askpass-env` then
# routes this session's ssh passphrase prompts through sshakku, so a key that
# has expired from the agent is asked about once, here, rather than by ssh on
# whatever it can find. All the logic lives in the core; this script only pins
# the session to the endpoint and invokes it.
#
# Where the Bourne hook can ask its shell whether anybody is sitting at it, this
# one has nothing to ask: PowerShell runs its profile for a session that was
# handed a command to run exactly as it does for a person's. So it is worked out
# below instead, before anything is run, and two things turn on the answer — the
# keys that are added, and what this session is told. `sshakku doctor` reports
# what is wired and what is not, in either kind of session.
#
# Every cmdlet below is named with its module and every type with its full
# namespace. This file is dot-sourced into whatever session the user has
# arranged for themselves, and there a function, an alias or a `using
# namespace` of the same name is reached before the built-in one: a hook that
# runs in someone else's session has to say exactly what it means.

# Invoke-Expression is what this hook exists to do: shell-init reports the
# resolved paths as assignments for the calling shell to run, exactly as the
# Bourne hook `eval`s them. Nothing else is evaluated, and the lines come from
# the binary named below rather than from anywhere a caller can reach.
[System.Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingInvokeExpression', '')]
param()

$sshakku_bin = '@SSHAKKU_BIN@'

# Whether somebody is sitting at this session. Two things below turn on it: the
# user's keys are loaded only where there is somebody to answer for them, and
# what the program has to say about a failure is put where it can be read only
# where there is somebody to read it. This shell runs its profile for a session
# that was handed a command or a script to run as well — a build step, a
# scheduled task, a `git` helper — and neither a question nor a diagnostic
# belongs in one of those, whose output is something else's input.
#
# There is no single flag to read for it, the way a Bourne shell has one, so the
# two things that can be known are both asked. The first is this session's own
# invocation: one given a command, a file or an encoded command to run, or told
# outright it is not interactive, is not one to ask anything of. The names may be
# abbreviated to any prefix, which is why they are matched as prefixes. The
# second is where its input comes from: a session reading its commands from a
# pipe or a file is being driven by something rather than by somebody, and that
# is true even when nothing on the command line said so.
$sshakku_interactive = $true
foreach ($sshakku_arg in [System.Environment]::GetCommandLineArgs()) {
    if (-not $sshakku_arg.StartsWith('-')) { continue }
    $sshakku_flag = $sshakku_arg.TrimStart('-').Split(':')[0].ToLowerInvariant()
    if ($sshakku_flag.Length -eq 0) { continue }
    foreach ($sshakku_batch in @('command', 'file', 'encodedcommand', 'noninteractive')) {
        if ($sshakku_batch.StartsWith($sshakku_flag)) { $sshakku_interactive = $false }
    }
}
if ([System.Console]::IsInputRedirected) { $sshakku_interactive = $false }

# Resolve the runtime paths, keep the agent healthy, and evaluate the printed
# assignments. Declare them first so an absent or failing binary leaves them
# empty rather than undefined.
$agent_sock = ''
$log_file = ''
if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $sshakku_bin -PathType Leaf) {
    # Only standard output is evaluated, and never as anything but the
    # assignments it is: what the binary has to say about a failure it says on
    # standard error instead, where no line this session would run can come
    # from.
    #
    # What belongs on that stream is the binary's judgement rather than this
    # hook's — what reaches it is what somebody is meant to act on, and what
    # needs no acting on it keeps to the session log. All that is decided here
    # is whether there is a somebody: a session a person opened is left to see
    # it, and one that was handed work is not.
    #
    # Where it is seen, it is left alone rather than captured and written out
    # again. Standard error read back into this shell arrives as an error
    # record, and that is two hazards at once: it is rendered wrapped to the
    # width of the console, which breaks a long message across lines and leaves
    # a command named in one no longer a command anybody can paste; and in
    # Windows PowerShell, in a session that has asked for errors to stop it, it
    # ends the profile it is being read in.
    $sshakku_previous_exit = $LASTEXITCODE
    if ($sshakku_interactive) {
        $init = & $sshakku_bin shell-init --shell=powershell
    } else {
        $init = & $sshakku_bin shell-init --shell=powershell 2>$null
    }
    $sshakku_ok = $LASTEXITCODE -eq 0
    # Put the exit code back the way it was found. A prompt that shows the last
    # command's status is a common setup, and a hook that runs before the first
    # prompt of every session must not be what that status reports on.
    $global:LASTEXITCODE = $sshakku_previous_exit
    if ($sshakku_ok -and $init) {
        Microsoft.PowerShell.Utility\Invoke-Expression ($init -join [System.Environment]::NewLine)
    }
}
# Without the resolved paths there is nothing we can safely do.
if (-not $agent_sock -or -not $log_file) {
    return
}

# Always pin this session to the fixed endpoint, so every ssh started from it
# reaches the agent sshakku keeps healthy rather than one of its own.
$env:SSH_AUTH_SOCK = $agent_sock
# Assigning $null to an environment variable removes it, which is what is
# wanted: a stale SSH_AGENT_PID inherited from elsewhere names an agent this
# session is no longer talking to.
$env:SSH_AGENT_PID = $null

# Point this session's ssh passphrase prompts at sshakku. Every session gets
# them, since these are two environment assignments and nothing else: what they
# say is where ssh should go when it needs a passphrase, and a session that
# never needs one is unaffected. The exit code is put back for the same reason
# it is above — a hook that runs before the first prompt must not be what the
# prompt reports on.
$sshakku_previous_exit = $LASTEXITCODE
$askpass_env = & $sshakku_bin askpass-env --shell=powershell 2>$null
$sshakku_ok = $LASTEXITCODE -eq 0
$global:LASTEXITCODE = $sshakku_previous_exit
if ($sshakku_ok -and $askpass_env) {
    Microsoft.PowerShell.Utility\Invoke-Expression ($askpass_env -join [System.Environment]::NewLine)
}

# Load the user's keys, but only in a session somebody is sitting at, which was
# worked out at the top. Loading may ask for a passphrase and write to the
# terminal, and in a session that was handed work to do a question stops that
# work at a prompt nobody is watching.
if ($sshakku_interactive) {
    $sshakku_previous_exit = $LASTEXITCODE
    & $sshakku_bin load-keys
    # Loading keys is something this session does for the user, not something
    # the user asked for, so what it returns must not become what their next
    # prompt reports on.
    $global:LASTEXITCODE = $sshakku_previous_exit
}
