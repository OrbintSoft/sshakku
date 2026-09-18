#!/usr/bin/env pwsh
#
# Verifies F63 against the real program: an OpenSSH too old to be sent to a
# passphrase helper is named in the report, and one new enough is not mentioned
# at all.
#
# What this covers that a unit test cannot is the walk out of the program and
# back — deciding which `ssh` this session would run, starting it, and reading
# what it answers on the stream it answers on. The tests in the suite hand the
# version in at the seam and go no further, so every step before that seam is
# exercised here or nowhere.
#
# The release matters because of what happens below it without a word. Older
# builds have no SSH_ASKPASS_REQUIRE, and reach a passphrase helper only where
# DISPLAY names an X server; an ordinary Windows session names none, so the
# wallet is never consulted however correctly everything else is wired, and the
# passphrase is asked for on the terminal instead.
#
# Both halves run against a stand-in rather than against whatever OpenSSH the
# machine happens to carry: the answer has to be the script's to choose, or the
# silent half would be reporting the runner's version rather than testing for
# silence.
#
# Nothing is installed and nothing on the host is touched: the program under
# test is built into <work_dir> and run from there, and `doctor` is a look
# rather than an action.
#
# Usage: windows-ssh-version-smoke.ps1 <work_dir>
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

# Two directories: the program under test, and the one put in front of PATH
# holding the stand-in. Keeping them apart means the helper this program looks
# for beside itself is not accidentally the stand-in, and vice versa.
$bin = Microsoft.PowerShell.Management\Join-Path -Path $WorkDir -ChildPath 'bin'
$front = Microsoft.PowerShell.Management\Join-Path -Path $WorkDir -ChildPath 'front'
foreach ($dir in @($bin, $front)) {
    Microsoft.PowerShell.Management\New-Item -ItemType Directory -Force -Path $dir |
        Microsoft.PowerShell.Core\Out-Null
}

$sshakku = Microsoft.PowerShell.Management\Join-Path -Path $bin -ChildPath 'sshakku.exe'
$standIn = Microsoft.PowerShell.Management\Join-Path -Path $front -ChildPath 'ssh.exe'

& go build -o $sshakku (Microsoft.PowerShell.Management\Join-Path -Path $repoRoot -ChildPath 'cmd/sshakku')
if ($LASTEXITCODE -ne 0) {
    throw "building sshakku.exe failed with exit code $LASTEXITCODE"
}

& go build -o $standIn (
    Microsoft.PowerShell.Management\Join-Path -Path $repoRoot -ChildPath 'test/fakes/openssh-version.go')
if ($LASTEXITCODE -ne 0) {
    throw "building the OpenSSH stand-in failed with exit code $LASTEXITCODE"
}

# The report's findings and nothing else. Read as a section rather than by
# searching the whole report, because the report ends with a tail of the session
# log — and a line an earlier run wrote there would otherwise answer the
# question the findings are being asked, which is how a check like this passes
# on a machine somebody has been working on and fails on a fresh one.
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

# Runs doctor with the stand-in first on PATH, claiming the given release, and
# returns the findings. The PATH is put back whatever happens, so a failure
# partway through cannot leave the rest of the script testing something else.
function Get-FindingsAgainstVersion {
    param([string] $Claimed)

    $savedPath = $env:PATH
    $savedVersion = $env:SSHAKKU_TEST_SSH_VERSION
    try {
        $env:PATH = "$front;$savedPath"
        $env:SSHAKKU_TEST_SSH_VERSION = $Claimed
        $report = (& $sshakku doctor 2>&1 |
            Microsoft.PowerShell.Utility\Out-String -Width 400)
        return Get-FindingSection -Report $report
    } finally {
        $env:PATH = $savedPath
        $env:SSHAKKU_TEST_SSH_VERSION = $savedVersion
    }
}

# The stand-in has to be the ssh this session finds, or both halves below pass
# by testing the machine's own OpenSSH twice.
$savedPath = $env:PATH
try {
    $env:PATH = "$front;$savedPath"
    $found = (Microsoft.PowerShell.Core\Get-Command -Name 'ssh' -ErrorAction SilentlyContinue).Source
} finally {
    $env:PATH = $savedPath
}
if ($found -ne $standIn) {
    $failures.Add("precondition: this session resolves ssh to '$found', not to the stand-in at '$standIn'")
}

# ---------------------------------------------------------------------------
# A build from before the release that made this possible.
# ---------------------------------------------------------------------------
Microsoft.PowerShell.Utility\Write-Output '--- against OpenSSH_8.1p1 ---'

$old = Get-FindingsAgainstVersion -Claimed 'OpenSSH_8.1p1, OpenSSL 1.0.2s  28 May 2019'
$named = $old |
    Microsoft.PowerShell.Core\Where-Object { $_ -match 'OpenSSH_8\.1p1' } |
    Microsoft.PowerShell.Utility\Select-Object -First 1

if (-not $named) {
    $failures.Add('doctor does not name the build it found')
} else {
    if ($named -notmatch '8\.4') {
        $failures.Add('doctor names the build but not the release it would take, leaving nothing to compare it with')
    }
    # What the build printed after the comma is the crypto library it was
    # linked against, which has no part in this decision and no business in a
    # sentence a reader is weighing an ssh upgrade with.
    if ($named -match 'OpenSSL') {
        $failures.Add('doctor quotes the crypto library alongside the build, in a sentence about ssh')
    }
}

# ---------------------------------------------------------------------------
# A build that can be told. The same question, so that the answer above cannot
# be had by the program simply saying this to everyone.
# ---------------------------------------------------------------------------
Microsoft.PowerShell.Utility\Write-Output '--- against OpenSSH_for_Windows_9.5p2 ---'

$current = Get-FindingsAgainstVersion -Claimed 'OpenSSH_for_Windows_9.5p2, LibreSSL 3.8.2'
$mentioned = $current |
    Microsoft.PowerShell.Core\Where-Object { $_ -match 'OpenSSH_|needs 8\.4' }
if ($mentioned) {
    $failures.Add("doctor says something about a version that is new enough: $mentioned")
}

# And it is still reporting: a report that had stopped producing findings
# altogether would pass the check above for the wrong reason.
if ($current.Count -eq 0) {
    $failures.Add('doctor produced no findings at all, so the silence above says nothing')
}

# ---------------------------------------------------------------------------
Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F63 holds on both sides of the release it turns on'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
