#!/usr/bin/env pwsh
#
# A key that has run out of time, on a machine whose agent expires nothing.
#
# Verifies F53: a key does not outlive the lifetime it was given, even where the
# agent it was handed to cannot expire anything — the session that opens next
# takes it out — and a key somebody added themselves is left alone whatever its
# age. The agent here keeps what it is given in the registry, so without this a
# key survives the session, the agent and the reboot.
#
# What is arranged, and what is not. The agent service is enabled and started
# first: whether a session can start one is F51's promise and is verified in its
# own scenario, and a machine with no agent has nothing to take a key out of.
# The keys are put in the agent with ssh-add, and one of them is then recorded
# as one SSHakku added and gave a lifetime to. That last part is arranged
# because it cannot be driven here: this platform has no wallet yet, so the only
# way SSHakku obtains a passphrase is by asking on a console, and a container has
# nobody to type into one — the same limit the test matrix records against the
# keys themselves.
#
# What is not arranged is the whole of what is being judged: a real session
# opening, a real agent, a real ssh-add, and what the session log says
# afterwards. Nothing here tells the product which key to take out, or that it
# should take one out at all.
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

# A key of this account's own, with no passphrase on it: what a passphrase is
# worth is not what this scenario is about, and a key that opens without one is
# a key ssh-add takes without asking anybody anything.
function Initialize-Key {
    param([string] $Path)

    $null = Invoke-Native -FilePath 'ssh-keygen.exe' -Arguments @(
        '-t', 'ed25519', '-N', '""', '-C', 'sshakku-expiry-scenario', '-f', $Path, '-q')
    $shown = (Invoke-Native -FilePath 'ssh-keygen.exe' -Arguments @('-lf', "$Path.pub")).Out
    if ("$shown" -match '(?<fp>SHA256:\S+)') {
        return $Matches['fp']
    }
    $failures.Add("no fingerprint could be read for the key at $Path, so nothing about it can be checked in the agent")
    return ''
}

# What the agent is holding, as the agent itself says it.
function Get-AgentListing {
    $listed = Invoke-Native -FilePath 'ssh-add.exe' -Arguments @('-l')
    "$($listed.Out)$($listed.Err)"
}

# A session as a person opens one: a new process that reads the profile the
# install wrote, and so runs the product on its way in.
function Open-Session {
    Invoke-Native -FilePath 'powershell.exe' -Arguments @(
        '-Command', 'Microsoft.PowerShell.Utility\Write-Output "opened"')
}

# What SSHakku writes down as it adds a key: when it was added, how long it is
# meant to stay, and the file it came from. Written here because a container has
# no way to reach the moment SSHakku would write it (see the header), and
# backdated rather than waited out, so the scenario spends no time asleep.
#
# This is the one piece of the product's own bookkeeping this scenario knows the
# shape of. That it does is a cost, not a convenience: were the shape to change,
# what is seeded here stops being read, and this scenario fails rather than
# passing on a state nothing recognises.
function Write-KeyRecord {
    param(
        [string] $Dir,
        [string] $KeyFile,
        [datetime] $AddedAt,
        [int] $LifetimeSeconds
    )

    if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $Dir -PathType Container)) {
        Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $Dir | Microsoft.PowerShell.Core\Out-Null
    }
    $record = Microsoft.PowerShell.Management\Join-Path -Path $Dir -ChildPath (
        Microsoft.PowerShell.Management\Split-Path -Path $KeyFile -Leaf)
    Microsoft.PowerShell.Management\Set-Content -LiteralPath $record -Value @(
        $AddedAt.ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
        [string] $LifetimeSeconds
        $KeyFile
    )
    $record
}

Microsoft.PowerShell.Utility\Write-Output '--- a machine with an agent running ---'

Microsoft.PowerShell.Management\Set-Service -Name ssh-agent -StartupType Automatic
Microsoft.PowerShell.Management\Start-Service -Name ssh-agent
$status = (Microsoft.PowerShell.Management\Get-Service -Name ssh-agent).Status.ToString()
Microsoft.PowerShell.Utility\Write-Output "ssh-agent: $status"
if ($status -ne 'Running') {
    $failures.Add("precondition: the ssh-agent service is $status, so there is no agent to take a key out of")
}

$install = Invoke-Native -FilePath $Sshakku -Arguments @('install', '--shell', 'windowspowershell')
if ($install.ExitCode -ne 0) {
    $failures.Add("install exited $($install.ExitCode), so no session below is a wired one")
    Microsoft.PowerShell.Utility\Write-Output $install.Err
}

# Where this account's records and session log live is the product's to say, not
# this scenario's to assemble: the lock and the log are printed as paths, and the
# records sit beside the lock.
$init = Invoke-Native -FilePath $Sshakku -Arguments @('shell-init', '--shell', 'powershell')
$lock = ''
$log = ''
if ($init.Out -match "agent_lock\s*=\s*'(?<path>[^']+)'") { $lock = $Matches['path'] }
if ($init.Out -match "log_file\s*=\s*'(?<path>[^']+)'") { $log = $Matches['path'] }
if (-not $lock -or -not $log) {
    Microsoft.PowerShell.Utility\Write-Output $init.Out
    $failures.Add('shell-init named no lock or no session log, so the directories of this account cannot be found')
}
$keystate = Microsoft.PowerShell.Management\Join-Path -Path (
    Microsoft.PowerShell.Management\Split-Path -Path $lock -Parent) -ChildPath 'keystate'

Microsoft.PowerShell.Utility\Write-Output '--- three keys in the agent ---'

$sshDir = Microsoft.PowerShell.Management\Join-Path -Path $env:USERPROFILE -ChildPath '.ssh'
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $sshDir -PathType Container)) {
    Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $sshDir | Microsoft.PowerShell.Core\Out-Null
}
$expiring = Microsoft.PowerShell.Management\Join-Path -Path $sshDir -ChildPath 'id_expiring'
$fresh = Microsoft.PowerShell.Management\Join-Path -Path $sshDir -ChildPath 'id_fresh'
$byHand = Microsoft.PowerShell.Management\Join-Path -Path $sshDir -ChildPath 'id_by_hand'

$expiringFP = Initialize-Key -Path $expiring
$freshFP = Initialize-Key -Path $fresh
$byHandFP = Initialize-Key -Path $byHand

foreach ($key in @($expiring, $fresh, $byHand)) {
    $added = Invoke-Native -FilePath 'ssh-add.exe' -Arguments @($key)
    if ($added.ExitCode -ne 0) {
        $failures.Add("the agent would not take $key : $("$($added.Err)".Trim())")
    }
}

# One key ran out of time an hour ago, one has seven hours left, and one was
# never SSHakku's to begin with.
$null = Write-KeyRecord -Dir $keystate -KeyFile $expiring `
    -AddedAt ([System.DateTime]::UtcNow.AddHours(-9)) -LifetimeSeconds 28800
$null = Write-KeyRecord -Dir $keystate -KeyFile $fresh `
    -AddedAt ([System.DateTime]::UtcNow.AddHours(-1)) -LifetimeSeconds 28800

$before = Get-AgentListing
Microsoft.PowerShell.Utility\Write-Output "the agent holds:`n$($before.Trim())"
foreach ($pair in @(@('id_expiring', $expiringFP), @('id_fresh', $freshFP), @('id_by_hand', $byHandFP))) {
    if ($pair[1] -and $before -notmatch [System.Text.RegularExpressions.Regex]::Escape($pair[1])) {
        $failures.Add("precondition: $($pair[0]) never reached the agent, so what becomes of it proves nothing")
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- a session opens ---'

$session = Open-Session
Microsoft.PowerShell.Utility\Write-Output "a session opened with exit $($session.ExitCode)"
if ($session.ExitCode -ne 0) {
    $failures.Add("a session that had a key to expire exited $($session.ExitCode) rather than opening")
}

$after = Get-AgentListing
Microsoft.PowerShell.Utility\Write-Output "the agent now holds:`n$($after.Trim())"

if ($expiringFP -and $after -match [System.Text.RegularExpressions.Regex]::Escape($expiringFP)) {
    $failures.Add('a key past its lifetime was still in the agent after a session had opened')
}
if ($freshFP -and $after -notmatch [System.Text.RegularExpressions.Regex]::Escape($freshFP)) {
    $failures.Add('a key still within its lifetime was taken out of the agent')
}
if ($byHandFP -and $after -notmatch [System.Text.RegularExpressions.Regex]::Escape($byHandFP)) {
    $failures.Add('a key added by hand, which SSHakku never recorded, was taken out of the agent')
}

# The record goes with the key: one left behind has every later session asking
# the agent for a key it has not got.
$expiringRecord = Microsoft.PowerShell.Management\Join-Path -Path $keystate -ChildPath 'id_expiring'
if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $expiringRecord -PathType Leaf) {
    $failures.Add('the record for a key that was taken out is still there')
}

Microsoft.PowerShell.Utility\Write-Output '--- what the session log holds ---'

if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $log -PathType Leaf)) {
    $failures.Add("shell-init named $log as the session log, which is not there")
} else {
    $lines = Microsoft.PowerShell.Management\Get-Content -LiteralPath $log
    Microsoft.PowerShell.Utility\Write-Output ($lines -join [System.Environment]::NewLine)

    $said = @($lines | Microsoft.PowerShell.Core\Where-Object {
            $_ -match 'id_expiring' -and $_ -match 'out of the agent' })
    if ($said.Count -eq 0) {
        $failures.Add('the session log does not name the key the session took out of the agent')
    }
    $wrongly = @($lines | Microsoft.PowerShell.Core\Where-Object {
            $_ -match 'id_by_hand' -or ($_ -match 'id_fresh' -and $_ -match 'out of the agent') })
    if ($wrongly.Count -ne 0) {
        $failures.Add("the session log names a key it had no business touching: $($wrongly -join '; ')")
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F53 holds on a machine whose agent keeps a key until something removes it'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
