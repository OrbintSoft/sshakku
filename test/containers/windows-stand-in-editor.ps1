#!/usr/bin/env pwsh
#
# A stand-in for the user's editor, for windows-config-edit-scenario.ps1: a
# program SSHakku starts the way it starts any editor, which writes down the
# file it was asked to open and then saves something into it.
#
# It is started as `powershell -File <this>`, which is a program with arguments
# of its own — the shape the setting under test is written in — and it is put
# under a path with a space in it by the scenario, which is the half of that
# shape a command line cut on spaces cannot express.
#
# Driven by the environment rather than by arguments of its own, so that
# everything on the command line is SSHakku's:
#
#   SSHAKKU_TEST_EDITOR_RECORD   file to append the opened path to
#   SSHAKKU_TEST_EDITOR_SAVES    file whose lines are appended to the opened
#                                file; unset leaves it exactly as it was found
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

param(
    [Parameter(Mandatory)]
    [string] $Path
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($env:SSHAKKU_TEST_EDITOR_RECORD) {
    Microsoft.PowerShell.Management\Add-Content -LiteralPath $env:SSHAKKU_TEST_EDITOR_RECORD -Value $Path
}

if ($env:SSHAKKU_TEST_EDITOR_SAVES) {
    Microsoft.PowerShell.Management\Add-Content -LiteralPath $Path -Value (
        Microsoft.PowerShell.Management\Get-Content -LiteralPath $env:SSHAKKU_TEST_EDITOR_SAVES)
}

exit 0
