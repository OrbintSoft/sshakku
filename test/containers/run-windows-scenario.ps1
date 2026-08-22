#!/usr/bin/env pwsh
#
# Runs one scenario against the real program, on a Windows machine that has
# never been touched — see windows-servercore.Dockerfile for why that needs a
# container rather than a directory.
#
# The program is built from the tree this script lives in, so a scenario is
# always run against the working copy rather than against whatever was
# installed on the machine running it.
#
# Usage:
#   ./run-windows-scenario.ps1 windows-fresh-install-scenario.ps1
#   ./run-windows-scenario.ps1 <scenario> -Cli docker -Isolation process
#
# -Cli names the container command. nerdctl and docker are both drivers of the
# same containerd underneath and take the same arguments for what is done here,
# so a machine with either can run this.
#
# -Isolation says how the container is kept apart from the host. `process` is
# faster and needs the host's build to be exactly the base image's; `hyperv`
# gives the container a kernel of its own and does not care, which is what any
# machine whose build has moved on from the image's needs. The default is the
# one that works anywhere.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string] $Scenario,

    [ValidateSet('nerdctl', 'docker')]
    [string] $Cli = 'nerdctl',

    [ValidateSet('hyperv', 'process')]
    [string] $Isolation = 'hyperv',

    [string] $Tag = 'sshakku-windows-test:latest'
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$containers = $PSScriptRoot
$repoRoot = Microsoft.PowerShell.Management\Split-Path -Path (
    Microsoft.PowerShell.Management\Split-Path -Path $containers -Parent) -Parent

$scenarioPath = Microsoft.PowerShell.Management\Join-Path -Path $containers -ChildPath $Scenario
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $scenarioPath -PathType Leaf)) {
    throw "no such scenario: $scenarioPath"
}

# A directory of its own, so what the build can see is only what is put here.
# The built program must not be left in the tree, where the next build of it
# for another platform would find it.
$context = Microsoft.PowerShell.Management\Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath ([System.Guid]::NewGuid().ToString('n'))
Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $context | Microsoft.PowerShell.Core\Out-Null

try {
    Microsoft.PowerShell.Utility\Write-Output "building the program under test for windows/amd64"
    $binary = Microsoft.PowerShell.Management\Join-Path -Path $context -ChildPath 'sshakku.exe'
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    & go build -o $binary (Microsoft.PowerShell.Management\Join-Path -Path $repoRoot -ChildPath 'cmd/sshakku')
    if ($LASTEXITCODE -ne 0) {
        throw "building sshakku.exe failed with exit code $LASTEXITCODE"
    }

    Microsoft.PowerShell.Management\Copy-Item -Path (
        Microsoft.PowerShell.Management\Join-Path -Path $containers -ChildPath 'windows-servercore.Dockerfile') -Destination $context
    Microsoft.PowerShell.Management\Copy-Item -Path (
        Microsoft.PowerShell.Management\Join-Path -Path $containers -ChildPath 'windows-*-scenario.ps1') -Destination $context

    Microsoft.PowerShell.Utility\Write-Output "building the image with $Cli"
    & $Cli build --tag $Tag --file (
        Microsoft.PowerShell.Management\Join-Path -Path $context -ChildPath 'windows-servercore.Dockerfile') $context
    if ($LASTEXITCODE -ne 0) {
        throw "building the image failed with exit code $LASTEXITCODE"
    }

    Microsoft.PowerShell.Utility\Write-Output "running $Scenario under $Isolation isolation"
    & $Cli run --rm --isolation $Isolation $Tag `
        powershell -NoProfile -ExecutionPolicy Bypass -File "C:\scenario\$Scenario"
    $scenarioExit = $LASTEXITCODE
} finally {
    Microsoft.PowerShell.Management\Remove-Item -LiteralPath $context -Recurse -Force -ErrorAction SilentlyContinue
}

exit $scenarioExit
