#!/usr/bin/env pwsh
#
# Verifies F62 against the real program on Windows: a copy of SSHakku whose
# askpass helper is not beside it says so, and says it in the one place a
# session can act on, rather than letting OpenSSH turn the absence into a wrong
# passphrase.
#
# This is worth a test of its own because of how it fails without one. ssh-add
# takes a passphrase only from the program named in SSH_ASKPASS, and OpenSSH
# meeting a name that resolves to nothing hands ssh-add an empty passphrase
# instead of reporting the program — so the key is refused, the refusal reads as
# a wrong passphrase, the stored one is written off as stale, and the key is
# finally given up on. Every part of that names something other than the cause.
#
# Nothing is installed and nothing on the host is touched: the program under
# test is built into <work_dir> and run from there, and the two commands driven
# here are the two that only read — `askpass-env` prints lines for a shell, and
# `doctor` is a look rather than an action.
#
# Usage: windows-askpass-helper-smoke.ps1 <work_dir>
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string] $WorkDir
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest

# Native programs report failure through their exit code, which is read
# explicitly below; a preference that turned their standard error into a
# terminating error would end the run before those codes could be looked at.
$ErrorActionPreference = 'Continue'

$repoRoot = Microsoft.PowerShell.Management\Split-Path -Path (
    Microsoft.PowerShell.Management\Split-Path -Path $PSCommandPath -Parent) -Parent

$failures = [System.Collections.Generic.List[string]]::new()

# A directory of its own, holding the program and nothing else, so that what is
# beside it is only ever what this script put there.
$bin = Microsoft.PowerShell.Management\Join-Path -Path $WorkDir -ChildPath 'bin'
Microsoft.PowerShell.Management\New-Item -ItemType Directory -Force -Path $bin |
    Microsoft.PowerShell.Core\Out-Null

$sshakku = Microsoft.PowerShell.Management\Join-Path -Path $bin -ChildPath 'sshakku.exe'
$helper = Microsoft.PowerShell.Management\Join-Path -Path $bin -ChildPath 'sshakku-askpass.exe'

& go build -o $sshakku (Microsoft.PowerShell.Management\Join-Path -Path $repoRoot -ChildPath 'cmd/sshakku')
if ($LASTEXITCODE -ne 0) {
    throw "building sshakku.exe failed with exit code $LASTEXITCODE"
}

# The precondition is asserted rather than assumed: were the helper already
# beside the program, the first half below would pass and would have tested
# nothing.
if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $helper) {
    $failures.Add("precondition: $helper is already there, so this is not a program without its helper")
}

function Get-Output {
    param([string[]] $SshakkuArgs)

    $text = (& $sshakku @SshakkuArgs 2>&1 |
        Microsoft.PowerShell.Utility\Out-String -Width 400)
    return [PSCustomObject]@{ Text = $text; Exit = $LASTEXITCODE }
}

# The report's findings and nothing else. Read as a section rather than by
# searching the whole report, because the report ends with a tail of the session
# log — and a line this very program wrote there on an earlier run would
# otherwise answer the question the findings are being asked, which is how a
# check like this passes on a machine somebody has been working on and fails on
# a fresh one.
function Get-FindingSection {
    param([string] $Report)

    $inSection = $false
    $found = [System.Collections.Generic.List[string]]::new()
    foreach ($line in ($Report -split "`r?`n")) {
        if ($line -match '^findings:') { $inSection = $true; continue }
        if ($inSection -and $line -match '^\S') { break }
        if ($inSection -and $line.Trim() -ne '') { $found.Add($line) }
    }
    return $found
}

# ---------------------------------------------------------------------------
# Without the helper.
# ---------------------------------------------------------------------------
Microsoft.PowerShell.Utility\Write-Output '--- with no helper beside the program ---'

$exports = Get-Output -SshakkuArgs @('askpass-env', '--shell=powershell')
Microsoft.PowerShell.Utility\Write-Output "askpass-env exit $($exports.Exit), output: [$($exports.Text.Trim())]"

# F62: the shell is left as it would be on a machine where SSHakku was never
# installed, which is a shell that can still be asked for a passphrase. Handing
# it SSH_ASKPASS_REQUIRE=force here would take its own prompt away and put
# nothing in its place.
if ($exports.Exit -ne 0) {
    $failures.Add("askpass-env exited $($exports.Exit); a shell that cannot be wired this way still opens")
}
if ($exports.Text.Trim() -ne '') {
    $failures.Add('askpass-env printed exports pointing at a helper that is not there')
}

$report = Get-Output -SshakkuArgs @('doctor')
$named = Get-FindingSection -Report $report.Text |
    Microsoft.PowerShell.Core\Where-Object { $_ -match 'sshakku-askpass' } |
    Microsoft.PowerShell.Utility\Select-Object -First 1

if (-not $named) {
    $failures.Add('doctor does not name the program that is missing')
} else {
    # The one thing this finding must never say — and it is read off the
    # finding itself rather than off the whole report, because the advice is
    # right where the other findings give it: an agent a session has not been
    # pointed at really is picked up by opening one. A file that is not there
    # is not, so the reader would come back to the same report.
    foreach ($sendsThemAway in @('starting a new session', 'new login shell', 'log out')) {
        if ($named -match [System.Text.RegularExpressions.Regex]::Escape($sendsThemAway)) {
            $failures.Add("doctor tells the reader to '$sendsThemAway', which cannot put a missing file back")
        }
    }
}

# ---------------------------------------------------------------------------
# With it. The same two questions, so that neither answer above can be had by
# the program simply never saying anything.
# ---------------------------------------------------------------------------
Microsoft.PowerShell.Management\Copy-Item -LiteralPath $sshakku -Destination $helper
Microsoft.PowerShell.Utility\Write-Output '--- with the helper beside the program ---'

$exports = Get-Output -SshakkuArgs @('askpass-env', '--shell=powershell')
Microsoft.PowerShell.Utility\Write-Output "askpass-env exit $($exports.Exit), output: [$($exports.Text.Trim())]"

if ($exports.Text -notmatch [System.Text.RegularExpressions.Regex]::Escape($helper)) {
    $failures.Add('askpass-env does not point ssh at the helper that is there')
}
if ($exports.Text -notmatch 'SSH_ASKPASS_REQUIRE') {
    $failures.Add('askpass-env does not force ssh to consult it, so a session with no display would not')
}

$report = Get-Output -SshakkuArgs @('doctor')
$stillNamed = Get-FindingSection -Report $report.Text |
    Microsoft.PowerShell.Core\Where-Object { $_ -match 'is not beside this one' }
if ($stillNamed) {
    $failures.Add('doctor reports a helper that is there as missing')
}

# ---------------------------------------------------------------------------
Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F62 holds for a program with and without its askpass helper'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
