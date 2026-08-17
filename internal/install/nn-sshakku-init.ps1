# SSH bootstrap, dot-sourced from a PowerShell profile — per-user or
# all-users, Windows PowerShell 5.x or PowerShell 7 and later. A profile that
# reads a Profile.d directory reaches this same installed copy through the
# small wrapper dropped in there.
#
# This is a thin hook around the sshakku core. `sshakku shell-init`, evaluated
# below, keeps an ssh-agent healthy on a fixed endpoint and prints the runtime
# paths to use; the fixed endpoint means the SSH_AUTH_SOCK this session gets
# never goes stale even if the agent is restarted. All the logic lives in the
# core; this script only pins the session to the endpoint and invokes it.
#
# Two things the Bourne hook does are deliberately absent here, and their
# absence is not an oversight: routing ssh passphrase prompts through the
# sshakku askpass broker, and adding the user's keys in interactive sessions.
# Neither is supported on this platform — `sshakku doctor` reports what is.
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

# Resolve the runtime paths, keep the agent healthy, and evaluate the printed
# assignments. Declare them first so an absent or failing binary leaves them
# empty rather than undefined.
$agent_sock = ''
$log_file = ''
if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $sshakku_bin -PathType Leaf) {
    # Only standard output is evaluated. What the binary has to say about a
    # failure goes to its standard error and to the session log, which is
    # where it can be read — not into every new prompt, and never into a line
    # this session would try to run.
    $sshakku_previous_exit = $LASTEXITCODE
    $init = & $sshakku_bin shell-init --shell=powershell 2>$null
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
