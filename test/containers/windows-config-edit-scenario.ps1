#!/usr/bin/env pwsh
#
# `sshakku config --edit` met on a machine nobody has set up: no editor named
# in any configuration, neither $EDITOR nor $VISUAL set, and — since this is a
# Windows Server image with no desktop — no Notepad on it at all.
#
# Verifies F36, in the order a person meets it. First the editor nobody named:
# an editor is opened on the user's own file, whatever editors this particular
# Windows turns out to carry, rather than the command exiting with the name of
# a program that was never here. Then the editor they did name, written the way
# a program on this system has to be written — a path with a space in it, in
# quotes, with arguments after it. Then that the configuration is what decides,
# even where the environment names something else.
#
# The editor is a program that waits for somebody, so the first round starts it
# without waiting, reads which program it is and which file it was handed off
# the process itself, and then ends it. Nothing here types into an editor: what
# is being checked is which editor was opened and on what, which is the whole
# of what SSHakku decides.
#
# Every program below is run through Start-Process rather than the call
# operator, and its output read back from files. A native program's standard
# error reaching this shell directly is rendered as an error record and wrapped
# to the width of a console, which splits a one-line message into two and would
# have this scenario judging its own formatting rather than what was written.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

param(
    [string] $Sshakku = 'C:\sshakku-under-test\sshakku.exe',
    [string] $ScenarioDir = 'C:\scenario'
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$failures = [System.Collections.Generic.List[string]]::new()

# Where the editor the scenario names is put: a directory with a space in its
# name, which is where this system installs programs and what the quoting in
# windows-config-edit-config.toml is there for.
$editorHome = 'C:\Program Files\SSHakku Test'

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

# The report's line for one setting, which carries the value in force and what
# decided it.
function Get-SettingLine {
    param(
        [string] $Report,
        [string] $Key
    )

    foreach ($line in ($Report -split "`n")) {
        if ($line.Trim().StartsWith($Key)) { return $line.Trim() }
    }
    return ''
}

Microsoft.PowerShell.Utility\Write-Output '--- the machine this starts on ---'

# Which editors this Windows carries is the point of the first round rather
# than an assumption of it, so it is read and printed. What the scenario does
# need is that it carries at least one, since a machine with none could not
# tell an editor that was not opened from one there was none to open.
$carried = @()
foreach ($editor in @('notepad++.exe', 'edit.exe', 'notepad.exe')) {
    $found = Microsoft.PowerShell.Core\Get-Command $editor -ErrorAction SilentlyContinue
    if ($found) {
        $carried += $editor
        Microsoft.PowerShell.Utility\Write-Output "  $editor -> $($found.Source)"
    } else {
        Microsoft.PowerShell.Utility\Write-Output "  $editor -> not on this machine"
    }
}
if ($carried.Count -eq 0) {
    $failures.Add('this machine carries no editor at all, so nothing here could tell an editor that was not opened from one there was none to open')
}

$env:EDITOR = ''
$env:VISUAL = ''

Microsoft.PowerShell.Utility\Write-Output '--- what the report says will be opened ---'

$report = Invoke-Native -FilePath $Sshakku -Arguments @('config')
if ($report.ExitCode -ne 0) {
    $failures.Add("a report reads and changes nothing and cannot fail, and this exited $($report.ExitCode): $($report.Err)")
}

$configDir = ''
foreach ($line in ($report.Out -split "`n")) {
    if ($line -match '^config directory:\s*(.+?)\s*$') { $configDir = $Matches[1] }
}
if (-not $configDir) {
    $failures.Add('the report does not name the directory it reads, so this scenario cannot find the file it edits')
}
Microsoft.PowerShell.Utility\Write-Output "config directory: $configDir"

$editorLine = Get-SettingLine -Report $report.Out -Key 'editor'
Microsoft.PowerShell.Utility\Write-Output "the report's own line: $editorLine"
if (-not $editorLine) {
    $failures.Add('the report carries no line for the editor, so nothing says which one would be opened')
}
if ($editorLine -notmatch 'default') {
    $failures.Add('the editor nobody named must be attributed to nothing but the default')
}
$named = $carried | Microsoft.PowerShell.Core\Where-Object { $editorLine -match [System.Text.RegularExpressions.Regex]::Escape($_) }
if (-not $named) {
    $failures.Add("the report names an editor this machine has not got: $editorLine")
}

Microsoft.PowerShell.Utility\Write-Output '--- the editor nobody named ---'

# Started without waiting: an editor waits for the person who opened it, and
# what is under test is which one was opened and on what. A window of its own
# rather than this shell's console, so that a console editor drawing itself
# does not draw over the output this scenario is judged on.
$editing = Microsoft.PowerShell.Management\Start-Process -FilePath $Sshakku -ArgumentList @('config', '--edit') -PassThru

$opened = $null
$deadline = [System.DateTime]::UtcNow.AddSeconds(30)
while (-not $opened -and [System.DateTime]::UtcNow -lt $deadline) {
    # Not every child is the editor: a process given a window of its own is
    # given a console host with it, which arrives first and is this system's
    # rather than SSHakku's.
    $opened = CimCmdlets\Get-CimInstance -ClassName Win32_Process -Filter "ParentProcessId=$($editing.Id)" |
        Microsoft.PowerShell.Core\Where-Object { $_.Name -ne 'conhost.exe' } |
        Microsoft.PowerShell.Utility\Select-Object -First 1
    if (-not $opened) {
        if ($editing.HasExited) { break }
        Microsoft.PowerShell.Utility\Start-Sleep -Milliseconds 250
    }
}

if ($opened) {
    Microsoft.PowerShell.Utility\Write-Output "it opened: $($opened.CommandLine)"
    if ($opened.CommandLine -notmatch 'config\.toml') {
        $failures.Add("the editor was opened on something other than the user's own file: $($opened.CommandLine)")
    }
    Microsoft.PowerShell.Management\Stop-Process -Id $opened.ProcessId -Force -ErrorAction SilentlyContinue
    # What SSHakku exits with after its editor was killed says nothing about
    # this promise, so it is waited for and not judged.
    $editing | Microsoft.PowerShell.Management\Wait-Process -Timeout 30 -ErrorAction SilentlyContinue
} else {
    $failures.Add('no editor was opened at all on a machine that carries one')
    # Said again with its output kept, since nothing was started that could
    # draw over it: the message names the program that was looked for, which is
    # the whole of what went wrong.
    $said = Invoke-Native -FilePath $Sshakku -Arguments @('config', '--edit')
    Microsoft.PowerShell.Utility\Write-Output "instead, it exited $($said.ExitCode) saying: $($said.Err)"
}

Microsoft.PowerShell.Utility\Write-Output '--- the editor named in the configuration ---'

Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $editorHome -Force | Microsoft.PowerShell.Core\Out-Null
Microsoft.PowerShell.Management\Copy-Item `
    -LiteralPath (Microsoft.PowerShell.Management\Join-Path -Path $ScenarioDir -ChildPath 'windows-stand-in-editor.ps1') `
    -Destination $editorHome -Force

Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $configDir -Force | Microsoft.PowerShell.Core\Out-Null
Microsoft.PowerShell.Management\Copy-Item `
    -LiteralPath (Microsoft.PowerShell.Management\Join-Path -Path $ScenarioDir -ChildPath 'windows-config-edit-config.toml') `
    -Destination (Microsoft.PowerShell.Management\Join-Path -Path $configDir -ChildPath 'config.toml') -Force

$record = Microsoft.PowerShell.Management\Join-Path -Path $env:TEMP -ChildPath 'opened.txt'
Microsoft.PowerShell.Management\Remove-Item -LiteralPath $record -Force -ErrorAction SilentlyContinue
$env:SSHAKKU_TEST_EDITOR_RECORD = $record
$env:SSHAKKU_TEST_EDITOR_SAVES = Microsoft.PowerShell.Management\Join-Path -Path $ScenarioDir -ChildPath 'windows-config-edit-saved.toml'

$edited = Invoke-Native -FilePath $Sshakku -Arguments @('config', '--edit')
if ($edited.ExitCode -ne 0) {
    $failures.Add("an editor named under a path with a space in it must still be run, and this exited $($edited.ExitCode): $($edited.Err)")
}
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $record)) {
    $failures.Add('the editor named in the configuration was never run')
} else {
    $wasOpened = Microsoft.PowerShell.Management\Get-Content -LiteralPath $record -Raw
    Microsoft.PowerShell.Utility\Write-Output "the editor named was handed: $($wasOpened.Trim())"
    if ($wasOpened -notmatch 'config\.toml') {
        $failures.Add("the editor was handed something other than the user's own file: $wasOpened")
    }
}

# What that editor saved is what SSHakku reads afterwards, which is the half of
# the promise an editor that merely started does not keep.
$after = Invoke-Native -FilePath $Sshakku -Arguments @('config')
$lifetime = Get-SettingLine -Report $after.Out -Key 'key_lifetime'
Microsoft.PowerShell.Utility\Write-Output "after the edit: $lifetime"
if ($lifetime -notmatch '3h') {
    $failures.Add("what the editor saved is not what SSHakku then read: $lifetime")
}

$editorLine = Get-SettingLine -Report $after.Out -Key 'editor'
Microsoft.PowerShell.Utility\Write-Output "the report's own line: $editorLine"
if ($editorLine -notmatch 'config\.toml') {
    $failures.Add("the report does not say the configuration is what named the editor: $editorLine")
}

Microsoft.PowerShell.Utility\Write-Output '--- the configuration decides, not the environment ---'

$env:EDITOR = 'sshakku-no-such-editor'
Microsoft.PowerShell.Management\Remove-Item -LiteralPath $record -Force -ErrorAction SilentlyContinue

# This round is about which editor ran, and the file it is handed already holds
# what the last one saved: saving the same setting into it again would leave a
# key defined twice, which SSHakku is right to refuse and which says nothing
# about the editor that wrote it.
$env:SSHAKKU_TEST_EDITOR_SAVES = ''

$again = Invoke-Native -FilePath $Sshakku -Arguments @('config', '--edit')
if ($again.ExitCode -ne 0) {
    $failures.Add("an editor named in the configuration must be run whatever the environment says, and this exited $($again.ExitCode): $($again.Err)")
}
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $record)) {
    $failures.Add('the environment was read in place of the configuration, so the editor named there never ran')
} else {
    Microsoft.PowerShell.Utility\Write-Output 'the editor named in the configuration ran again, with $EDITOR naming a program that is not here'
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'

if ($failures.Count -gt 0) {
    foreach ($failure in $failures) {
        Microsoft.PowerShell.Utility\Write-Output "FAIL: $failure"
    }
    Microsoft.PowerShell.Utility\Write-Output "$($failures.Count) failure(s)"
    exit 1
}

Microsoft.PowerShell.Utility\Write-Output 'PASS'
exit 0
