#!/usr/bin/env pwsh
#
# Puts KeePassXC's command-line tool into the build context of
# windows-keepassxc.Dockerfile, so the image can COPY it in.
#
# It runs on the host and not in the image, and that is a constraint rather
# than a preference: a `RUN` step on this harness executes under process
# isolation, which requires the host's Windows build to be exactly the base
# image's. It is not, and cannot be made to be, so a container started for a
# RUN exits before anything in it runs — `hcs::System::Start ... exited
# unexpectedly`, which names nothing about the command. That is why every
# Windows image here is COPY and nothing else, and why fetching anything an
# image needs happens out here.
#
# The portable archive rather than the installer: it is the same binaries, and
# what a container wants is files in a directory rather than a program
# registered with the machine. Nothing here needs KeePassXC to be installed in
# the sense Windows means — the cli route runs keepassxc-cli against a database
# file and talks to no running instance.
#
# The download is pinned to a version and checked against the digest that
# version was published with. An image whose tools change underneath it turns
# every later failure into a question about which of two things moved, and the
# archive comes over the network, which is exactly where a check belongs.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

[CmdletBinding()]
param(
    # The build context to prepare. The runner passes the directory it is about
    # to hand the container CLI.
    [Parameter(Mandatory)]
    [string] $Context,

    [string] $Version = '2.7.12',

    # The SHA-256 the project published beside the archive, in its .DIGEST file.
    [string] $Sha256 = '958234b0669d757b53eacf42bdd5de0fa1cc1ab7527709ddf4f7e29c06a8305f'
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Windows PowerShell negotiates whatever the platform defaults to, which on an
# older host is a protocol GitHub no longer answers; the failure is a closed
# connection rather than anything naming TLS.
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12

$archive = "KeePassXC-$Version-Win64.zip"
$url = "https://github.com/keepassxreboot/keepassxc/releases/download/$Version/$archive"

# Kept between runs, since it is 30-odd megabytes of somebody else's release and
# it is pinned: the digest below decides whether what is here is usable, so a
# cached copy is checked exactly as hard as a fresh one.
$cache = Microsoft.PowerShell.Management\Join-Path -Path ([System.IO.Path]::GetTempPath()) `
    -ChildPath 'sshakku-container-downloads'
Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $cache -Force | Microsoft.PowerShell.Core\Out-Null
$download = Microsoft.PowerShell.Management\Join-Path -Path $cache -ChildPath $archive

function Test-Digest {
    param([string] $Path)

    if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }
    (Microsoft.PowerShell.Utility\Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -eq $Sha256.ToUpperInvariant()
}

if (Test-Digest -Path $download) {
    Microsoft.PowerShell.Utility\Write-Output "using the copy already fetched, which matches the published digest"
} else {
    Microsoft.PowerShell.Utility\Write-Output "fetching $url"
    # -UseBasicParsing because a host without Internet Explorer has nothing for
    # the other kind to build its document with, and without it the call fails
    # on a machine perfectly able to fetch the file.
    Microsoft.PowerShell.Utility\Invoke-WebRequest -Uri $url -OutFile $download -UseBasicParsing
    if (-not (Test-Digest -Path $download)) {
        $got = (Microsoft.PowerShell.Utility\Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $download -Force -ErrorAction SilentlyContinue
        throw "$archive hashes to $got, and the version pinned here was published as $Sha256"
    }
    Microsoft.PowerShell.Utility\Write-Output 'digest matches the published one'
}

$destination = Microsoft.PowerShell.Management\Join-Path -Path $Context -ChildPath 'KeePassXC'
Microsoft.PowerShell.Archive\Expand-Archive -LiteralPath $download -DestinationPath $destination -Force

# The archive unpacks into a directory named for the release. Everything is
# lifted out of it so the tool sits at a path that does not carry a version,
# which is what the image and every scenario can then name.
$unpacked = Microsoft.PowerShell.Management\Get-ChildItem -LiteralPath $destination -Directory |
    Microsoft.PowerShell.Utility\Select-Object -First 1
if ($unpacked) {
    Microsoft.PowerShell.Management\Move-Item -Path (
        Microsoft.PowerShell.Management\Join-Path -Path $unpacked.FullName -ChildPath '*') -Destination $destination
    Microsoft.PowerShell.Management\Remove-Item -LiteralPath $unpacked.FullName -Recurse -Force
}

$cli = Microsoft.PowerShell.Management\Join-Path -Path $destination -ChildPath 'keepassxc-cli.exe'
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $cli -PathType Leaf)) {
    throw "the archive unpacked without a keepassxc-cli.exe at $cli"
}

# KeePassXC is built with Microsoft's C++ compiler and its archive ships none of
# that compiler's runtime, because on a desktop the redistributable is already
# installed. A Server Core image has no such thing, and what happens there is
# not an error message: the loader cannot resolve the imports, the process dies
# before main, and both its streams are empty — so every call reads as
# keepassxc-cli refusing rather than as keepassxc-cli never having started.
#
# They are copied from this machine because there is nowhere in the image to
# install them from: an image here can run no program of its own. Nothing is
# added to the repository by this — the files are fetched at build time onto a
# local image that is never published, exactly as KeePassXC itself is.
$runtime = @(
    'vcruntime140.dll', 'vcruntime140_1.dll', 'vcruntime140_threads.dll',
    'msvcp140.dll', 'msvcp140_1.dll', 'msvcp140_2.dll',
    'msvcp140_atomic_wait.dll', 'msvcp140_codecvt_ids.dll', 'concrt140.dll'
)
$system32 = Microsoft.PowerShell.Management\Join-Path -Path $env:SystemRoot -ChildPath 'System32'
$missing = [System.Collections.Generic.List[string]]::new()
foreach ($dll in $runtime) {
    $source = Microsoft.PowerShell.Management\Join-Path -Path $system32 -ChildPath $dll
    if (Microsoft.PowerShell.Management\Test-Path -LiteralPath $source -PathType Leaf) {
        Microsoft.PowerShell.Management\Copy-Item -LiteralPath $source -Destination $destination -Force
    } else {
        $missing.Add($dll)
    }
}
if ($missing.Count -gt 0) {
    throw ("this machine has no $($missing -join ', ') to give the image; " +
        'install the Microsoft Visual C++ 2015-2022 redistributable (x64) and run this again')
}

Microsoft.PowerShell.Utility\Write-Output "KeePassXC $Version and the C++ runtime it needs are in the build context"
