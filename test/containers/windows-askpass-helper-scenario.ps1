#!/usr/bin/env pwsh
#
# A key whose passphrase is in the wallet, on a machine whose askpass helper is
# not where it belongs.
#
# Verifies F62, and it is the only place the whole of it can be seen, because
# what F62 exists to prevent is not a message but a chain of them. ssh-add takes
# a passphrase from one program alone, and OpenSSH meeting a name that resolves
# to nothing does not report the program: it hands ssh-add an empty passphrase.
# From there every step is reasonable and every step is wrong — the key is
# refused, the refusal reads as a wrong passphrase, the passphrase that came out
# of the wallet is written off as stale, the user is asked as often as they
# allowed, and the key is finally given up on and skipped in every later shell.
# None of that can be shown against a fake ssh-add, because a fake one would
# have to be told to behave this way, which is the behaviour under test.
#
# So the helper is taken away from a machine that is otherwise entirely correct:
# a real agent, a real ssh-add, a real wallet holding the right passphrase. Then
# it is put back and the same command is run again, because a first half that
# ends in "no key was loaded" proves nothing unless the second half shows a key
# that loads.
#
# The passphrase carries no spaces or quotes on purpose. `cmdkey` parses its own
# command line and does not take the quoting a caller would apply, which would
# have this scenario measuring an argument-passing convention rather than a
# wallet.
#
# Every program below is run through Start-Process rather than the call
# operator, and its output read back from files. A native program's standard
# error reaching this shell directly is rendered as an error record and wrapped
# to the width of a console, which splits a one-line message into two and would
# have this scenario judging its own formatting rather than what was written.
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

$passphrase = 'correct-horse-battery-staple'
$keyName = 'id_askpass'
$service = "SSHakku-Key-$keyName"

# Where the helper belongs: beside the program, under the name that program
# looks for. Taking it aside and putting it back is what this scenario does to
# the machine, and it is undone before the scenario ends.
$helper = Microsoft.PowerShell.Management\Join-Path `
    -Path (Microsoft.PowerShell.Management\Split-Path -Path $Sshakku -Parent) `
    -ChildPath 'sshakku-askpass.exe'
$helperAside = "$helper.aside"

# Runs a program and hands back what it wrote and what it exited with, exactly
# as it wrote it.
#
# The wait is bounded, and that is not caution: it is the assertion. Without the
# check F62 promises, a wallet passphrase read as wrong sends SSHakku on to ask
# a person, and a container has a console with nobody at it — so the program
# waits for someone who will never type. Killed here, that is a failure with a
# name instead of a run that hangs until something outside ends it.
function Invoke-Native {
    param(
        [string] $FilePath,
        [string[]] $Arguments,
        [int] $TimeoutSeconds = 120
    )

    $outFile = [System.IO.Path]::GetTempFileName()
    $errFile = [System.IO.Path]::GetTempFileName()
    try {
        $p = Microsoft.PowerShell.Management\Start-Process -FilePath $FilePath `
            -ArgumentList $Arguments -NoNewWindow -PassThru `
            -RedirectStandardOutput $outFile -RedirectStandardError $errFile
        # Reading the handle is what makes the exit code readable afterwards.
        # Start-Process caches it only when asked, and a process object that was
        # never asked reports no exit code at all once the process is gone —
        # which reads exactly like a program that exited 0.
        $null = $p.Handle
        $timedOut = -not $p.WaitForExit($TimeoutSeconds * 1000)
        if ($timedOut) {
            $p.Kill()
            $null = $p.WaitForExit(5000)
        }
        [PSCustomObject]@{
            Out      = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $outFile -Raw)
            Err      = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $errFile -Raw)
            ExitCode = $(if ($timedOut) { -1 } else { $p.ExitCode })
            TimedOut = $timedOut
        }
    } finally {
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $outFile, $errFile -Force -ErrorAction SilentlyContinue
    }
}

# A key of this account's own, locked with a passphrase — which is the point
# here: a key that opens without one never reaches the helper at all.
function Initialize-Key {
    # The analyzer's objection to a passphrase in a plain string is right about
    # a program and wrong about this: the passphrase here is a fixture, known to
    # anyone reading the file, and it has to reach ssh-keygen as characters.
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingPlainTextForPassword', 'Passphrase',
        Justification = 'A throwaway fixture passphrase, which has to be handed to ssh-keygen as characters.')]
    param(
        [string] $Path,
        [string] $Passphrase
    )

    $null = Invoke-Native -FilePath 'ssh-keygen.exe' -Arguments @(
        '-t', 'ed25519', '-N', $Passphrase, '-C', 'sshakku-askpass-scenario', '-f', $Path, '-q')
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

# Puts a passphrase in the store the way anything but SSHakku would, so what is
# read back had to survive being written by one program and read by another.
function Write-StoreEntry {
    param(
        [string] $Target,
        [string] $User,
        [string] $Secret
    )

    $written = Invoke-Native -FilePath 'cmdkey.exe' -Arguments @(
        "/generic:$Target", "/user:$User", "/pass:$Secret")
    if ($written.ExitCode -ne 0) {
        $failures.Add("precondition: the store would not take $Target : $("$($written.Err)".Trim())")
    }
}

# The session log as it stands, from line $From on — so what one run wrote is
# read without the run before it.
function Get-LogSince {
    param([string] $Path, [int] $From)

    if (-not $Path -or -not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $Path -PathType Leaf)) {
        return @()
    }
    $all = @(Microsoft.PowerShell.Management\Get-Content -LiteralPath $Path)
    if ($From -ge $all.Count) { return @() }
    return $all[$From..($all.Count - 1)]
}

function Measure-LogLine {
    param([string] $Path)

    if (-not $Path -or -not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $Path -PathType Leaf)) {
        return 0
    }
    return @(Microsoft.PowerShell.Management\Get-Content -LiteralPath $Path).Count
}

Microsoft.PowerShell.Utility\Write-Output '--- a machine that is right in every other way ---'

Microsoft.PowerShell.Management\Set-Service -Name ssh-agent -StartupType Automatic
Microsoft.PowerShell.Management\Start-Service -Name ssh-agent
$status = (Microsoft.PowerShell.Management\Get-Service -Name ssh-agent).Status.ToString()
Microsoft.PowerShell.Utility\Write-Output "ssh-agent: $status"
if ($status -ne 'Running') {
    $failures.Add("precondition: the ssh-agent service is $status, so there is nowhere for a key to be loaded to")
}

if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $helper -PathType Leaf)) {
    $failures.Add("precondition: $helper is not there to begin with, so taking it away proves nothing")
}

# Where this account's session log lives is the product's to say, not this
# scenario's to assemble. Asked while the helper is still in place.
$init = Invoke-Native -FilePath $Sshakku -Arguments @('shell-init', '--shell', 'powershell')
$log = ''
if ($init.Out -match "log_file\s*=\s*'(?<path>[^']+)'") { $log = $Matches['path'] }
if (-not $log) {
    Microsoft.PowerShell.Utility\Write-Output $init.Out
    $failures.Add('shell-init named no session log, so what the product wrote down cannot be read')
}

$sshDir = Microsoft.PowerShell.Management\Join-Path -Path $env:USERPROFILE -ChildPath '.ssh'
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $sshDir -PathType Container)) {
    Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $sshDir | Microsoft.PowerShell.Core\Out-Null
}
$keyFile = Microsoft.PowerShell.Management\Join-Path -Path $sshDir -ChildPath $keyName
$keyFP = Initialize-Key -Path $keyFile -Passphrase $passphrase

Write-StoreEntry -Target $service -User 'SSHakku-Key' -Secret $passphrase

$before = Get-AgentListing
if ($keyFP -and $before -match [System.Text.RegularExpressions.Regex]::Escape($keyFP)) {
    $failures.Add('precondition: the key was already in the agent, so its being there afterwards proves nothing')
}

try {
    Microsoft.PowerShell.Utility\Write-Output '--- with the helper taken away ---'

    Microsoft.PowerShell.Management\Move-Item -LiteralPath $helper -Destination $helperAside

    $mark = Measure-LogLine -Path $log
    $load = Invoke-Native -FilePath $Sshakku -Arguments @('load-keys') -TimeoutSeconds 90
    Microsoft.PowerShell.Utility\Write-Output "load-keys exited $($load.ExitCode)"
    Microsoft.PowerShell.Utility\Write-Output "load-keys said: $("$($load.Err)".Trim())"

    # The failure this promise is about is a program that goes on asking. With
    # nobody at this console, going on means never coming back.
    if ($load.TimedOut) {
        $failures.Add('load-keys never came back: with no program to ask through it went on to ask a person, on a machine with nobody to answer')
    }
    if ("$($load.Err)" -notmatch 'sshakku-askpass') {
        $failures.Add('load-keys did not name the program that is missing, which is the one thing a reader can act on')
    }

    $lines = Get-LogSince -Path $log -From $mark
    Microsoft.PowerShell.Utility\Write-Output ($lines -join [System.Environment]::NewLine)

    # The three misattributions, each checked for by name. Every one of them is
    # a true sentence about something that did not happen.
    $stale = @($lines | Microsoft.PowerShell.Core\Where-Object { $_ -match 'is stale' })
    if ($stale.Count -ne 0) {
        $failures.Add("the passphrase in the wallet was written off as stale: $($stale -join '; ')")
    }
    $asked = @($lines | Microsoft.PowerShell.Core\Where-Object { $_ -match 'prompting|attempt \d+/' })
    if ($asked.Count -ne 0) {
        $failures.Add("somebody was asked for a passphrase that could not have been used: $($asked -join '; ')")
    }
    $gaveUp = @($lines | Microsoft.PowerShell.Core\Where-Object { $_ -match 'giving up on' })
    if ($gaveUp.Count -ne 0) {
        $failures.Add("the key was given up on, which makes every later shell skip it: $($gaveUp -join '; ')")
    }

    $after = Get-AgentListing
    if ($keyFP -and $after -match [System.Text.RegularExpressions.Regex]::Escape($keyFP)) {
        $failures.Add('the key reached the agent with no program to ask a passphrase through, so this machine is not the one described')
    }
} finally {
    if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $helperAside -PathType Leaf) {
        Microsoft.PowerShell.Management\Move-Item -LiteralPath $helperAside -Destination $helper -Force
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- with it put back ---'

$mark = Measure-LogLine -Path $log
$load = Invoke-Native -FilePath $Sshakku -Arguments @('load-keys') -TimeoutSeconds 90
Microsoft.PowerShell.Utility\Write-Output "load-keys exited $($load.ExitCode)"

if ($load.TimedOut) {
    $failures.Add('load-keys never came back with the helper in place, so nothing above can be attributed to its absence')
} elseif ($load.ExitCode -ne 0) {
    $failures.Add("load-keys exited $($load.ExitCode) with the helper in place: $("$($load.Err)".Trim())")
}

$after = Get-AgentListing
Microsoft.PowerShell.Utility\Write-Output "the agent now holds:`n$($after.Trim())"
if (-not $keyFP) {
    $failures.Add('no fingerprint to look for, so nothing about the load can be checked')
} elseif ($after -notmatch [System.Text.RegularExpressions.Regex]::Escape($keyFP)) {
    $failures.Add('the key did not reach the agent even with the helper in place, so the first half showed nothing about the helper')
}

# And it got there the way it was meant to: out of the wallet, with nobody
# asked. A key added some other way would look identical in the listing above.
$lines = Get-LogSince -Path $log -From $mark
$fromWallet = @($lines | Microsoft.PowerShell.Core\Where-Object {
        $_ -match [System.Text.RegularExpressions.Regex]::Escape($keyName) -and $_ -match 'stored passphrase' })
if ($fromWallet.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output ($lines -join [System.Environment]::NewLine)
    $failures.Add('the session log does not say the passphrase came from the wallet')
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F62 holds — a missing askpass helper is reported as itself, and nothing else is blamed for it'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
