#!/usr/bin/env pwsh
#
# A key that loads with nothing typed, out of the wallet this system keeps.
#
# Verifies F54, and F5 and F9 on this platform for the first time: a passphrase
# kept in the Credential Manager is read back and used, the key reaches the
# agent with nobody asked anything, `sshakku doctor` names that wallet and says
# what guards it, and `sshakku forget --all` takes SSHakku's entries out of a
# store shared with every other program on the machine while leaving somebody
# else's exactly where it was (F27).
#
# What is arranged, and what is not. The agent service is enabled and started
# first — whether a session can start one is F51's promise, verified in its own
# scenario — and the passphrase is put into the wallet with this system's own
# `cmdkey`, since the one thing a container cannot supply is a person typing at
# a console, which is how a passphrase gets there the first time. That first
# time is therefore not what this scenario judges. Everything after it is: a
# real wallet, a real `ssh-add`, a real agent, and a key that had to come out of
# that wallet to get there, since with nobody to ask there is no other way in.
#
# Seeding through `cmdkey` rather than by writing the store directly is what
# makes it a check of the product rather than of itself: SSHakku's own writing
# is not involved, so the passphrase has to survive being written by one program
# and read by another, which is the whole of what "the system's own wallet"
# claims.
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

$passphrase = 'correct-horse-battery-staple'
$keyName = 'id_wallet'
$service = "SSHakku-Key-$keyName"
$stranger = 'SomebodyElses-Credential'

# Runs a program and hands back what it wrote and what it exited with, exactly
# as it wrote it.
#
# The wait is bounded, and that is not caution. The way this promise fails is by
# not returning: with no passphrase to be had, SSHakku asks for one on the
# console, and a container has a console with nobody at it — so the program
# waits for a person who will never type, and an unbounded scenario waits with
# it until something outside kills them both. Killed here instead, it is a
# failure with a name.
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
# here: a key that opens without one proves nothing about a wallet.
function Initialize-Key {
    # The analyzer's objection to a passphrase in a plain string is right about
    # a program and wrong about this: the passphrase here is a fixture, known to
    # anyone reading the file, and it has to reach ssh-keygen as characters.
    # A SecureString would be unwrapped a line later and would hide what this
    # scenario is deliberately showing.
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingPlainTextForPassword', 'Passphrase',
        Justification = 'A throwaway fixture passphrase, which has to be handed to ssh-keygen as characters.')]
    param(
        [string] $Path,
        [string] $Passphrase
    )

    $null = Invoke-Native -FilePath 'ssh-keygen.exe' -Arguments @(
        '-t', 'ed25519', '-N', $Passphrase, '-C', 'sshakku-wallet-scenario', '-f', $Path, '-q')
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

# What the store holds, asked of the system rather than of SSHakku: a report
# that its own entry is gone is not the same as the entry being gone.
function Get-StoreListing {
    $listed = Invoke-Native -FilePath 'cmdkey.exe' -Arguments @('/list')
    "$($listed.Out)$($listed.Err)"
}

# Puts a passphrase in the store the way anything but SSHakku would.
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

Microsoft.PowerShell.Utility\Write-Output '--- a machine with an agent running ---'

Microsoft.PowerShell.Management\Set-Service -Name ssh-agent -StartupType Automatic
Microsoft.PowerShell.Management\Start-Service -Name ssh-agent
$status = (Microsoft.PowerShell.Management\Get-Service -Name ssh-agent).Status.ToString()
Microsoft.PowerShell.Utility\Write-Output "ssh-agent: $status"
if ($status -ne 'Running') {
    $failures.Add("precondition: the ssh-agent service is $status, so there is nowhere for a key to be loaded to")
}

$install = Invoke-Native -FilePath $Sshakku -Arguments @('install', '--shell', 'windowspowershell')
if ($install.ExitCode -ne 0) {
    $failures.Add("install exited $($install.ExitCode), so no session below is a wired one")
    Microsoft.PowerShell.Utility\Write-Output $install.Err
}

# Where this account's session log lives is the product's to say, not this
# scenario's to assemble.
$init = Invoke-Native -FilePath $Sshakku -Arguments @('shell-init', '--shell', 'powershell')
$log = ''
if ($init.Out -match "log_file\s*=\s*'(?<path>[^']+)'") { $log = $Matches['path'] }
if (-not $log) {
    Microsoft.PowerShell.Utility\Write-Output $init.Out
    $failures.Add('shell-init named no session log, so what the product wrote down cannot be read')
}

Microsoft.PowerShell.Utility\Write-Output '--- a locked key, and its passphrase in the wallet ---'

$sshDir = Microsoft.PowerShell.Management\Join-Path -Path $env:USERPROFILE -ChildPath '.ssh'
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $sshDir -PathType Container)) {
    Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $sshDir | Microsoft.PowerShell.Core\Out-Null
}
$keyFile = Microsoft.PowerShell.Management\Join-Path -Path $sshDir -ChildPath $keyName
$keyFP = Initialize-Key -Path $keyFile -Passphrase $passphrase

Write-StoreEntry -Target $service -User 'SSHakku-Key' -Secret $passphrase
Write-StoreEntry -Target $stranger -User 'somebody' -Secret 'not-ours-to-touch'

$before = Get-AgentListing
if ($keyFP -and $before -match [System.Text.RegularExpressions.Regex]::Escape($keyFP)) {
    $failures.Add('precondition: the key was already in the agent, so its being there afterwards proves nothing')
}

Microsoft.PowerShell.Utility\Write-Output '--- the report, before anything is loaded ---'

$doctor = Invoke-Native -FilePath $Sshakku -Arguments @('doctor')
Microsoft.PowerShell.Utility\Write-Output $doctor.Out
if ("$($doctor.Out)" -notmatch 'credential-manager') {
    $failures.Add('doctor does not name the wallet the passphrases actually go into')
}
if ("$($doctor.Out)" -notmatch 'any program running as you') {
    $failures.Add('doctor names the wallet without saying what guards it, which is the half a reader would otherwise assume')
}

Microsoft.PowerShell.Utility\Write-Output '--- the key loads, with nobody asked anything ---'

$load = Invoke-Native -FilePath $Sshakku -Arguments @('load-keys') -TimeoutSeconds 60
Microsoft.PowerShell.Utility\Write-Output "load-keys exited $($load.ExitCode)"
if ($load.TimedOut) {
    $failures.Add('load-keys never came back: with the passphrase in the wallet there was nothing to ask anybody, so waiting means it did not find it')
} elseif ($load.ExitCode -ne 0) {
    $failures.Add("load-keys exited $($load.ExitCode): $("$($load.Err)".Trim())")
}
if ("$($load.Out)".Trim() -ne '') {
    $failures.Add("load-keys wrote to standard output, which a session evaluates: $("$($load.Out)".Trim())")
}

$after = Get-AgentListing
Microsoft.PowerShell.Utility\Write-Output "the agent now holds:`n$($after.Trim())"
if (-not $keyFP) {
    $failures.Add('no fingerprint to look for, so nothing about the load can be checked')
} elseif ($after -notmatch [System.Text.RegularExpressions.Regex]::Escape($keyFP)) {
    $failures.Add('a key whose passphrase was in the wallet did not reach the agent, and there was nobody to ask for it')
}

if ($log -and (Microsoft.PowerShell.Management\Test-Path -LiteralPath $log -PathType Leaf)) {
    $lines = Microsoft.PowerShell.Management\Get-Content -LiteralPath $log
    Microsoft.PowerShell.Utility\Write-Output ($lines -join [System.Environment]::NewLine)

    # Which way the key got in matters as much as its being in: a key added
    # some other way would look identical in the listing above.
    $fromWallet = @($lines | Microsoft.PowerShell.Core\Where-Object {
            $_ -match [System.Text.RegularExpressions.Regex]::Escape($keyName) -and $_ -match 'stored passphrase' })
    if ($fromWallet.Count -eq 0) {
        $failures.Add('the session log does not say the passphrase came from the wallet')
    }
    $asked = @($lines | Microsoft.PowerShell.Core\Where-Object { $_ -match 'prompting' })
    if ($asked.Count -ne 0) {
        $failures.Add("something was asked for, on a machine with nobody to ask: $($asked -join '; ')")
    }
} else {
    $failures.Add("shell-init named $log as the session log, which is not there")
}

Microsoft.PowerShell.Utility\Write-Output '--- forgetting takes ours and only ours ---'

$forget = Invoke-Native -FilePath $Sshakku -Arguments @('forget', '--all')
Microsoft.PowerShell.Utility\Write-Output "forget --all exited $($forget.ExitCode)"
if ($forget.ExitCode -ne 0) {
    $failures.Add("forget --all exited $($forget.ExitCode): $("$($forget.Err)".Trim())")
}

$store = Get-StoreListing
if ($store -match [System.Text.RegularExpressions.Regex]::Escape($service)) {
    $failures.Add('the entry SSHakku stored is still in the store after it reported forgetting everything')
}
if ($store -notmatch [System.Text.RegularExpressions.Regex]::Escape($stranger)) {
    $failures.Add('a credential another program saved was taken out of the store along with ours')
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F54 holds — a key loads out of the wallet this system keeps, and forgetting takes only what SSHakku put there'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
