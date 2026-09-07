#!/usr/bin/env pwsh
#
# The terminal that hands the shell it opens a command of its own.
#
# Verifies F60, and with it the half of F5 only a session can show — a key
# loading into a shell somebody opened, with nothing typed. A session a terminal
# opened is somebody's although its command line says it was handed work, and a
# session handed work that leaves when the work is done is not — though both are
# given a console of their own, and though the two command lines differ by a
# single flag.
#
# The two are opened in that order, and the order is part of the assertion. The
# agent holds nothing to begin with; the session shaped like a build step must
# leave it holding nothing; only then can the session shaped like a terminal be
# said to have loaded the key rather than to have found it already there. Each
# side is what makes the other worth anything.
#
# What is arranged: an agent to hold the key — whether a session can start one
# is F51's promise and has a scenario of its own — a wired shell, a key where
# this account keeps its keys, and that key's passphrase already in the store.
# The passphrase is put there because "loads it silently, with nothing typed" is
# the promise being read: a session that had to be asked for one would be
# stopped at a question nothing here can answer, and what came of it would say
# nothing about which sessions are somebody's.
#
# What is not arranged is the whole of what is judged: how each session is
# started, and what the agent holds once it has run. Nothing here tells the
# product which of the two sessions is somebody's, because a session that had to
# be told would prove nothing about how the product tells them apart.
#
# Each session is started with nothing captured, which is what gives it a
# console of its own: Start-Process hands a child the standard handles this
# script was itself given the moment any stream is redirected, and those are
# pipes — which the product reads, rightly, as nobody being there. So a session
# writes down what it found for itself, from the inside, rather than being read
# over a pipe it would then be judged for having.
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

# A key of this account's own, locked with a passphrase, and the fingerprint the
# agent will name it by.
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
        '-t', 'ed25519', '-N', $Passphrase, '-C', 'sshakku-console-session-scenario', '-f', $Path, '-q')
    $shown = (Invoke-Native -FilePath 'ssh-keygen.exe' -Arguments @('-lf', "$Path.pub")).Out
    if ("$shown" -match '(?<fp>SHA256:\S+)') {
        return $Matches['fp']
    }
    $failures.Add("no fingerprint could be read for the key at $Path, so nothing about it can be checked in the agent")
    return ''
}

# Puts this key's passphrase in the store, so that a session with somebody at it
# has nothing to ask them.
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

# What the agent is holding, as the agent itself says it.
function Get-AgentListing {
    $listed = Invoke-Native -FilePath 'ssh-add.exe' -Arguments @('-l')
    "$($listed.Out)$($listed.Err)"
}

# Opens one session at a console of its own and hands back what that session
# said the agent was holding once the hook had run.
#
# -Stays is the whole difference between the two shapes. A terminal passes it,
# so the shell it opens goes on to a prompt when the command it was handed is
# finished; nothing that hands a shell work to do and waits for it ever passes
# it, since such a session would not end.
#
# The session is handed a command of its own either way, exactly as a terminal
# hands one, and that command is what ends the session: a shell told to stay
# would otherwise sit at a prompt nothing here can type into.
function Open-SessionAtAConsole {
    param([switch] $Stays)

    $said = Microsoft.PowerShell.Management\Join-Path -Path $env:TEMP `
        -ChildPath ("agent-as-this-session-found-it-" + [System.Guid]::NewGuid().ToString('N') + '.txt')

    $sessionArgs = @('-NoLogo')
    if ($Stays) {
        $sessionArgs += '-NoExit'
    }
    $sessionArgs += @('-Command', "& ssh-add.exe -l *> '$said'; [System.Environment]::Exit(0)")

    $session = Microsoft.PowerShell.Management\Start-Process -FilePath 'powershell.exe' `
        -ArgumentList $sessionArgs -WindowStyle Hidden -PassThru
    # Bounded, because a session that reaches its prompt is one nothing here can
    # type into: it would hold the whole run open rather than fail it.
    if (-not $session.WaitForExit(120000)) {
        $session.Kill()
        $failures.Add('a session opened at a console never ended')
        return ''
    }
    if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $said -PathType Leaf)) {
        $failures.Add('a session opened at a console wrote down nothing about what the agent was holding')
        return ''
    }
    try {
        return "$(Microsoft.PowerShell.Management\Get-Content -LiteralPath $said -Raw)"
    } finally {
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $said -Force -ErrorAction SilentlyContinue
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- the machine as it was found ---'
$agentService = CimCmdlets\Get-CimInstance -ClassName Win32_Service -Filter "Name='ssh-agent'"
$agentStatus = (Microsoft.PowerShell.Management\Get-Service -Name ssh-agent).Status
Microsoft.PowerShell.Utility\Write-Output "ssh-agent: $agentStatus, start mode $($agentService.StartMode)"

# An agent for the key to be loaded into. Arranged, because a machine with no
# agent has nothing to hold a key and both sessions would look alike.
Microsoft.PowerShell.Management\Set-Service -Name ssh-agent -StartupType Automatic
Microsoft.PowerShell.Management\Start-Service -Name ssh-agent

$install = Invoke-Native -FilePath $Sshakku -Arguments @('install', '--shell', 'windowspowershell')
if ($install.ExitCode -ne 0) {
    $failures.Add("install exited $($install.ExitCode), so no session below is a wired one")
    Microsoft.PowerShell.Utility\Write-Output $install.Err
}

$sshDir = Microsoft.PowerShell.Management\Join-Path -Path $env:USERPROFILE -ChildPath '.ssh'
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $sshDir -PathType Container)) {
    Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $sshDir | Microsoft.PowerShell.Core\Out-Null
}
$keyName = 'id_ed25519'
$passphrase = 'console-session-scenario-fixture'
$fingerprint = Initialize-Key -Passphrase $passphrase -Path (
    Microsoft.PowerShell.Management\Join-Path -Path $sshDir -ChildPath $keyName)
Write-StoreEntry -Target "SSHakku-Key-$keyName" -User 'SSHakku-Key' -Secret $passphrase
Microsoft.PowerShell.Utility\Write-Output "the key this account has: $fingerprint"

# Asserted rather than assumed: were the key already in the agent, both sessions
# below would find it there and neither would have tested anything.
$before = Get-AgentListing
Microsoft.PowerShell.Utility\Write-Output "the agent, before any session: $($before.Trim())"
if ($fingerprint -and $before -match [System.Text.RegularExpressions.Regex]::Escape($fingerprint)) {
    $failures.Add('precondition: the agent already holds this key, so no session below could be said to have loaded it')
}

Microsoft.PowerShell.Utility\Write-Output '--- a session handed work, which leaves when the work is done ---'
$afterWork = Open-SessionAtAConsole
Microsoft.PowerShell.Utility\Write-Output "it found the agent holding: $($afterWork.Trim())"
if ($fingerprint -and $afterWork -match [System.Text.RegularExpressions.Regex]::Escape($fingerprint)) {
    $failures.Add('a session that was handed work and leaves when it is done loaded the key anyway')
}

Microsoft.PowerShell.Utility\Write-Output '--- a session a terminal opened: handed a command of its own, and told to stay ---'
$afterTerminal = Open-SessionAtAConsole -Stays
Microsoft.PowerShell.Utility\Write-Output "it found the agent holding: $($afterTerminal.Trim())"
if ($fingerprint -and $afterTerminal -notmatch [System.Text.RegularExpressions.Regex]::Escape($fingerprint)) {
    $failures.Add('a session a terminal opened never loaded this account key, so nothing it starts can use it')
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: a shell a terminal opened with a command of its own gets its keys, and one handed work does not'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
