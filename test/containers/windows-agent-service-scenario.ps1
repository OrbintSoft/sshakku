#!/usr/bin/env pwsh
#
# SSHakku met on a machine whose agent is a service that is not running — the
# state a Windows Server image ships in, and one a machine somebody works on
# leaves behind the first time an agent is started on it.
#
# Verifies F51, in the two states it distinguishes and in the order a person
# meets them: first a service that cannot be started at all, then one that can.
# The command the first state names is not merely matched here, it is taken out
# of the message and run — a command named in full is one that can be pasted,
# and a described one is not, which is the whole of what that clause promises.
# Running it is also what puts the machine into the second state, so the two
# halves are one journey rather than two fixtures.
#
# What this cannot see, and does not claim to: F51 also promises that the line
# reaches the person whose shell it is. A session that is being driven by a
# script is not one somebody is sitting at, and this scenario is a script all
# the way down, so what it holds the product to is the session log — which the
# same promise names — and the message the command itself prints.
#
# Every program below is run through Start-Process rather than the call
# operator, and its output read back from files. A native program's standard
# error reaching this shell directly is rendered as an error record and wrapped
# to the width of a console, which splits a one-line message into two and would
# have this scenario judging its own formatting rather than what was written.
#
# `--shell windowspowershell` names the shell for `install`, which asks which
# shell to wire; `--shell powershell` names the dialect for `shell-init`, which
# asks how to spell what it prints. They are different questions.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

[CmdletBinding()]
param(
    [string] $Sshakku = 'C:\sshakku-under-test\sshakku.exe'
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$failures = [System.Collections.Generic.List[string]]::new()

# Runs a program and hands back what it wrote and what it exited with, exactly
# as it wrote it.
function Invoke-Native {
    param(
        [string] $FilePath,
        [string[]] $Arguments
    )

    $outFile = [System.IO.Path]::GetTempFileName()
    $errFile = [System.IO.Path]::GetTempFileName()
    try {
        $p = Microsoft.PowerShell.Management\Start-Process -FilePath $FilePath `
            -ArgumentList $Arguments -NoNewWindow -Wait -PassThru `
            -RedirectStandardOutput $outFile -RedirectStandardError $errFile
        [PSCustomObject]@{
            Out      = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $outFile -Raw)
            Err      = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $errFile -Raw)
            ExitCode = $p.ExitCode
        }
    } finally {
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $outFile, $errFile -Force -ErrorAction SilentlyContinue
    }
}

# Running the command the product named is the assertion, not a convenience:
# a command named in full is one that can be pasted into a shell, and no other
# check can tell that apart from a sentence describing what to do. What is run
# is whatever the message itself yielded, so nothing here can pass by naming a
# command of its own.
function Invoke-WhatItNamed {
    [System.Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingInvokeExpression', '')]
    param([string] $Command)

    Microsoft.PowerShell.Utility\Invoke-Expression -Command $Command
}

function Get-ServiceStartMode {
    (CimCmdlets\Get-CimInstance -ClassName Win32_Service -Filter "Name='ssh-agent'").StartMode
}

function Get-ServiceStatus {
    (Microsoft.PowerShell.Management\Get-Service -Name ssh-agent).Status.ToString()
}

# A session as a person opens one: a new process that reads the profile the
# install wrote. What it prints is the endpoint it was handed, so the caller can
# see what this session would give to ssh.
function Open-Session {
    Invoke-Native -FilePath 'powershell.exe' -Arguments @(
        '-Command', 'Microsoft.PowerShell.Utility\Write-Output "sock=[$env:SSH_AUTH_SOCK]"')
}

Microsoft.PowerShell.Utility\Write-Output '--- the machine as it was found ---'
$startMode = Get-ServiceStartMode
$status = Get-ServiceStatus
Microsoft.PowerShell.Utility\Write-Output "ssh-agent: $status, start mode $startMode"

# Asserted rather than assumed: were the service already running, everything
# below would pass and would have tested nothing.
if ($startMode -ne 'Disabled') {
    $failures.Add("precondition: the ssh-agent service starts $startMode, so this is not a machine whose agent cannot be started")
}
if ($status -ne 'Stopped') {
    $failures.Add("precondition: the ssh-agent service is $status, so there is no stopped service to start")
}

$install = Invoke-Native -FilePath $Sshakku -Arguments @('install', '--shell', 'windowspowershell')
if ($install.ExitCode -ne 0) {
    $failures.Add("install exited $($install.ExitCode), so no session below is a wired one")
    Microsoft.PowerShell.Utility\Write-Output $install.Err
}

Microsoft.PowerShell.Utility\Write-Output '--- a service that cannot be started ---'

# The shell still opens. Nothing about a broken agent may turn opening a
# terminal into a failure.
$session = Open-Session
Microsoft.PowerShell.Utility\Write-Output "a session opened with exit $($session.ExitCode) and said: $($session.Out.Trim())"

if ($session.ExitCode -ne 0) {
    $failures.Add("a session on a machine whose agent cannot be started exited $($session.ExitCode) rather than opening")
}

# And it is pointed at nothing rather than at silence: an endpoint handed to a
# session that nobody answers on is worse than none, because ssh then waits on
# it instead of asking.
if ($session.Out -notmatch 'sock=\[\]') {
    $failures.Add("a session was handed $($session.Out.Trim()) while no agent could be started")
}

if ((Get-ServiceStatus) -ne 'Stopped') {
    $failures.Add('a service that is disabled was reported as started')
}

# What the product says about it, in the words it says it in.
$refusal = (Invoke-Native -FilePath $Sshakku -Arguments @('shell-init', '--shell', 'powershell')).Err
Microsoft.PowerShell.Utility\Write-Output '--- what it says ---'
Microsoft.PowerShell.Utility\Write-Output $refusal

# "Named in full rather than described" means the command can be pasted, so it
# is pasted. Anything less than a runnable command fails to match, and a
# sentence about what an administrator ought to do would not survive being run.
$command = ''
if ($refusal -match '(?<cmd>Set-Service\s+ssh-agent\s+-StartupType\s+\w+)') {
    $command = $Matches['cmd']
}

if (-not $command) {
    $failures.Add('nothing in what it says is a command that could be run to put this right')
} else {
    Microsoft.PowerShell.Utility\Write-Output "--- running what it named: $command ---"
    Invoke-WhatItNamed -Command $command
    $nowStartMode = Get-ServiceStartMode
    if ($nowStartMode -eq 'Disabled') {
        $failures.Add("running the command it named left the service starting $nowStartMode")
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- a service that is not running ---'
Microsoft.PowerShell.Utility\Write-Output "ssh-agent: $(Get-ServiceStatus), start mode $(Get-ServiceStartMode)"

$session = Open-Session
Microsoft.PowerShell.Utility\Write-Output "a session opened with exit $($session.ExitCode) and said: $($session.Out.Trim())"

if ($session.ExitCode -ne 0) {
    $failures.Add("a session on a machine whose agent was stopped exited $($session.ExitCode) rather than opening")
}

# The service is running again, because the session that needed it started it.
$status = Get-ServiceStatus
if ($status -ne 'Running') {
    $failures.Add("a session that needed the agent left the service $status")
}

# And the session got a working agent: ssh-add answers. What it answers is not
# the point — an agent holding no keys says so — but being unable to reach one
# is, and that is the exit code this asks about.
$listed = Invoke-Native -FilePath 'ssh-add.exe' -Arguments @('-l')
Microsoft.PowerShell.Utility\Write-Output "ssh-add -l exited $($listed.ExitCode) saying: $("$($listed.Out)$($listed.Err)".Trim())"
if ($listed.ExitCode -eq 2) {
    $failures.Add('ssh-add could not reach an agent after a session had started one')
}

# The endpoint that session was handed is the one the system serves, not an
# empty string standing in for it.
if ($session.Out -match 'sock=\[(?<sock>.+)\]') {
    Microsoft.PowerShell.Utility\Write-Output "the session was pointed at $($Matches['sock'])"
} else {
    $failures.Add('a session with a running agent was pointed at nothing')
}

Microsoft.PowerShell.Utility\Write-Output '--- what the session log holds ---'

# The log names itself: `shell-init` prints where it is, now that it has an
# agent to report on, so nothing here has to know a path of its own.
$init = Invoke-Native -FilePath $Sshakku -Arguments @('shell-init', '--shell', 'powershell')
$log = ''
if ($init.Out -match "log_file\s*=\s*'(?<path>[^']+)'") {
    $log = $Matches['path']
}

if (-not $log) {
    $failures.Add('shell-init named no session log, so what it recorded cannot be read')
} elseif (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $log -PathType Leaf)) {
    $failures.Add("shell-init named $log as the session log, which is not there")
} else {
    $lines = Microsoft.PowerShell.Management\Get-Content -LiteralPath $log
    Microsoft.PowerShell.Utility\Write-Output ($lines -join [System.Environment]::NewLine)

    # F51 promises the log says the same thing the message did — the command,
    # in full, and not a note that something went wrong.
    $recorded = @($lines | Microsoft.PowerShell.Core\Where-Object {
            $_ -match 'Set-Service\s+ssh-agent\s+-StartupType' })
    if ($recorded.Count -eq 0) {
        $failures.Add('the session log does not name the command that puts a disabled agent right')
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F51 holds on a machine whose agent is a service that is not running'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
