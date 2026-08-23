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
# F51 also promises the line reaches the person whose shell it is, and says
# where: the screen of a shell somebody opened, the session log either way. Both
# sides are held here, because only together do they mean anything — a session
# opened at a console of its own must carry the sentence on its standard error,
# and one handed work on its command line must carry none of it, since a build
# step's own output is not a place to leave diagnostics.
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

# A session as a person opens one, and what it put on its screen.
#
# The two things this product reads to decide whether anybody is there are
# arranged rather than asserted — nothing on the command line saying the session
# was handed work, and a console of its own for standard input — because a
# session that had to be told it was a person's would prove nothing about how
# the product tells them apart.
#
# The console is why cmd is here, and it is the only reason. Asking Start-Process
# to capture a stream makes it hand the child the standard handles this scenario
# was itself given, and those are pipes: the session would read its input from
# one, which is precisely what the product reads to conclude that nobody is
# sitting there. Started with no stream captured, the session gets a console of
# its own — and cmd, which is what owns it, is asked for the one thing this shell
# then cannot do, to point that session's error stream at a file.
#
# Such a session is handed nothing to do, so it would sit at its prompt until
# something typed into it said otherwise, and nothing here can type. Saying it on
# the command line is not open to us either, so it is said in the one place a
# session reads before its first prompt: CurrentUserCurrentHost, the last of the
# four PowerShell reads, whichever of them the install wrote into. The process is
# ended rather than the shell asked to exit, because `exit` in a profile ends the
# profile and the session goes on to its prompt anyway. Everything read here was
# written before that: the hook runs ahead of the prompt, and what wrote it was a
# program that had already finished.
#
# That instruction is taken out again afterwards. It belongs to this one session,
# and a profile that keeps it makes every session after this one end before the
# work it was handed — including the ones the rest of this scenario opens.
#
# The wait is bounded because this is the state where the product may have
# nothing to say: a session that never reaches its profile's last line would
# otherwise hold the whole run open.
function Open-SessionAtAConsole {
    $profileFile = (Invoke-Native -FilePath 'powershell.exe' -Arguments @(
            '-NoProfile', '-Command', '[System.Console]::Out.Write($PROFILE)')).Out
    if (-not $profileFile) {
        $failures.Add('this host named no per-user profile, so a session opened at a console has no way to be told to leave')
        return
    }

    $dir = Microsoft.PowerShell.Management\Split-Path -Path $profileFile -Parent
    if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $dir -PathType Container)) {
        Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $dir | Microsoft.PowerShell.Core\Out-Null
    }
    $before = $null
    if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $profileFile -PathType Leaf) {
        $before = Microsoft.PowerShell.Management\Get-Content -LiteralPath $profileFile -Raw
    }

    $errFile = [System.IO.Path]::GetTempFileName()
    try {
        Microsoft.PowerShell.Management\Add-Content -LiteralPath $profileFile -Value '[System.Environment]::Exit(0)'
        $p = Microsoft.PowerShell.Management\Start-Process -FilePath 'cmd.exe' `
            -ArgumentList @('/c', "powershell.exe 2> `"$errFile`"") `
            -WindowStyle Hidden -PassThru
        if (-not $p.WaitForExit(60000)) {
            $p.Kill()
            $failures.Add('a session opened at a console never left, so what it said cannot be read')
        }
        [PSCustomObject]@{
            Err = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $errFile -Raw)
        }
    } finally {
        if ($null -eq $before) {
            Microsoft.PowerShell.Management\Remove-Item -LiteralPath $profileFile -Force -ErrorAction SilentlyContinue
        } else {
            Microsoft.PowerShell.Management\Set-Content -LiteralPath $profileFile -Value $before -NoNewline
        }
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $errFile -Force -ErrorAction SilentlyContinue
    }
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

# That session was handed its work on the command line, so nothing about a
# broken agent belongs in its output: whatever started it is capturing that,
# and a script that begins failing because SSHakku had something to say has
# been given a new problem in place of the one it was told about.
if ("$($session.Err)" -match 'Set-Service') {
    $failures.Add("a session handed work to run was given diagnostics on its standard error: $("$($session.Err)".Trim())")
}

Microsoft.PowerShell.Utility\Write-Output '--- a session somebody is sitting at ---'

$seated = Open-SessionAtAConsole
if ($null -ne $seated) {
    # Read as a string, and not as whatever the property holds. A session that
    # said nothing leaves that property empty, and an emptiness compared with
    # -notmatch answers with an empty collection rather than with true: the one
    # state this check exists to catch is the one it would let through.
    $said = "$($seated.Err)".Trim()
    Microsoft.PowerShell.Utility\Write-Output "it said, on its standard error: $said"

    # The one place this clause is about. A person whose agent cannot be started
    # is asked for every passphrase, every time; being told why, on the screen
    # they are looking at, is the whole of what F51 promises here.
    if ($said -notmatch 'Set-Service\s+ssh-agent\s+-StartupType') {
        $failures.Add('a session opened at a console was told nothing about an agent that could not be started')
    }
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
