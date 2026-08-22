#!/usr/bin/env pwsh
#
# SSHakku met by a Windows account that has never had a PowerShell profile —
# the state every machine is in before somebody works on it, and the one state
# a machine that has been worked on cannot be put back into.
#
# Verifies F44: `sshakku install` wires one shell in one command, and what it
# did is reported as things you can go and look at — the startup file it wrote
# into and the hook that file now runs. The report is read here the way a
# person reads it, and then the files it named are opened to see whether they
# say the same thing.
#
# `--shell windowspowershell` is named outright rather than left to the "the
# shell you ran it from" default, because what runs this script is a script
# host and not the interactive window that default is about.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

[CmdletBinding()]
param(
    [string] $Sshakku = 'C:\sshakku-under-test\sshakku.exe'
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest

# Native programs report failure through their exit code, which is read
# explicitly below; a preference that turned their standard error into a
# terminating error would end the run before those codes could be looked at.
$ErrorActionPreference = 'Continue'

$failures = [System.Collections.Generic.List[string]]::new()
$profilePath = $PROFILE.CurrentUserAllHosts

Microsoft.PowerShell.Utility\Write-Output "profile under test: $profilePath"

# The precondition is asserted rather than assumed: were the file already
# there, everything below would still pass and would have tested nothing.
if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $profilePath) {
    $failures.Add("precondition: $profilePath already exists, so this is not a machine nobody has touched")
}

$report = (& $Sshakku install --shell windowspowershell 2>&1 |
    Microsoft.PowerShell.Utility\Out-String).Trim()
$installExit = $LASTEXITCODE

Microsoft.PowerShell.Utility\Write-Output '--- what the install reported ---'
Microsoft.PowerShell.Utility\Write-Output $report
Microsoft.PowerShell.Utility\Write-Output "--- install exit code: $installExit ---"

if ($installExit -ne 0) {
    $failures.Add("install exited $installExit on a machine with no PowerShell profile directory")
}

# What F44 promises to report: the startup file it wrote into.
if ($report -notmatch [System.Text.RegularExpressions.Regex]::Escape($profilePath)) {
    $failures.Add('the report does not name the startup file it wrote into')
}

if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $profilePath)) {
    $failures.Add("$profilePath was not written")
} else {
    $lines = Microsoft.PowerShell.Management\Get-Content -LiteralPath $profilePath

    Microsoft.PowerShell.Utility\Write-Output '--- what the profile holds ---'
    Microsoft.PowerShell.Utility\Write-Output ($lines -join [System.Environment]::NewLine)

    # One block, however the file got here: F19's promise is that installing
    # adds SSHakku's own delimited block, and a second one would mean an
    # install that cannot be run twice.
    $opens = @($lines | Microsoft.PowerShell.Core\Where-Object { $_ -match '>>> sshakku >>>' }).Count
    if ($opens -ne 1) {
        $failures.Add("the profile holds $opens SSHakku blocks, not exactly one")
    }

    # The hook the file now runs has to be a file that is there. Its path is
    # read out of the block as the shell would read it: the quoted argument of
    # the dot-source line.
    $hook = $lines |
        Microsoft.PowerShell.Utility\Select-String -Pattern "^\s*\.\s+'(?<path>.+)'\s*$" |
        Microsoft.PowerShell.Core\ForEach-Object { $_.Matches[0].Groups['path'].Value } |
        Microsoft.PowerShell.Utility\Select-Object -First 1

    if (-not $hook) {
        $failures.Add('the profile holds no line dot-sourcing a hook file')
    } elseif (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $hook -PathType Leaf)) {
        $failures.Add("the profile dot-sources $hook, which is not there")
    } elseif ($report -notmatch [System.Text.RegularExpressions.Regex]::Escape($hook)) {
        $failures.Add("the report does not name $hook, the hook the profile now runs")
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F44 holds on a machine nobody has touched'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
