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
#   ./run-windows-scenario.ps1 windows-keepassxc-scenario.ps1 -Dockerfile windows-keepassxc.Dockerfile
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

    # Which machine to build. Which programs are present is itself under test —
    # one image has nothing installed on it, another carries a wallet — so a
    # scenario names the machine it needs rather than sharing one with every
    # other scenario.
    [string] $Dockerfile = 'windows-servercore.Dockerfile',

    [string] $Tag
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

$dockerfilePath = Microsoft.PowerShell.Management\Join-Path -Path $containers -ChildPath $Dockerfile
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $dockerfilePath -PathType Leaf)) {
    throw "no such image definition: $dockerfilePath"
}

# One tag per image. A shared default would have the second image overwrite the
# first under a name still claiming to be it, and the scenario that then ran
# would be a scenario run on the wrong machine — which fails in ways that look
# like the product's fault.
if (-not $Tag) {
    $Tag = 'sshakku-windows-test:latest'
    if ($Dockerfile -ne 'windows-servercore.Dockerfile') {
        $Tag = 'sshakku-' + [System.IO.Path]::GetFileNameWithoutExtension($Dockerfile) + ':latest'
    }
}

# containerd and buildkitd each serve a control pipe, and both are served to
# Administrators only. A session without that reaches neither, and what it is
# told is not what happened: the build reports that `buildctl` needs to be
# installed, when it is installed and was refused. Both are asked here, once and
# up front, so the answer names the thing that is actually wrong.
& $Cli images --quiet 2>&1 | Microsoft.PowerShell.Core\Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "$Cli cannot reach containerd. Its control pipe is served to Administrators only, so this needs an elevated session; check as well that the containerd service is running."
}

if ($Cli -eq 'nerdctl') {
    & buildctl debug workers 2>&1 | Microsoft.PowerShell.Core\Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw 'buildctl cannot reach buildkitd, which nerdctl builds through. The same elevation applies to its pipe; check as well that the buildkitd service is running, since it starts before containerd is ready unless it is made to depend on it.'
    }
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

    Microsoft.PowerShell.Management\Copy-Item -LiteralPath $dockerfilePath -Destination $context
    # Every Windows script and fixture, not only the scenarios: an image builds
    # itself with some of these and hands the rest to the scenario it runs.
    Microsoft.PowerShell.Management\Copy-Item -Path (
        Microsoft.PowerShell.Management\Join-Path -Path $containers -ChildPath 'windows-*.ps1') -Destination $context
    Microsoft.PowerShell.Management\Copy-Item -Path (
        Microsoft.PowerShell.Management\Join-Path -Path $containers -ChildPath 'windows-*.toml') -Destination $context

    # An image that needs something fetched says so by having a prepare script
    # beside it, and it is run out here rather than as a RUN step inside the
    # build. That is not a style choice: a RUN on Windows executes under process
    # isolation, which requires this host's build to be exactly the base image's.
    # Where it is not, the container for that step cannot start at all, and what
    # is reported — `hcs::System::Start ... exited unexpectedly` — names nothing
    # about the command it was going to run.
    $prepare = Microsoft.PowerShell.Management\Join-Path -Path $containers `
        -ChildPath ([System.IO.Path]::GetFileNameWithoutExtension($Dockerfile) + '-prepare.ps1')
    if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $prepare -PathType Leaf) {
        Microsoft.PowerShell.Utility\Write-Output "preparing the build context with $(Microsoft.PowerShell.Management\Split-Path -Path $prepare -Leaf)"
        & $prepare -Context $context
    }

    Microsoft.PowerShell.Utility\Write-Output "building $Tag from $Dockerfile with $Cli"
    & $Cli build --tag $Tag --file (
        Microsoft.PowerShell.Management\Join-Path -Path $context -ChildPath $Dockerfile) $context
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
