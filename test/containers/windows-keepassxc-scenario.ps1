#!/usr/bin/env pwsh
#
# A key that loads with nothing typed, out of a KeePassXC database, on Windows.
#
# Verifies F22 on this platform — KeePassXC chosen by name on every OS SSHakku
# supports — together with F5 and F9 through that wallet, F24 for a database
# that has nothing to ask about, and F27 for what `forget --all` leaves alone.
#
# What is arranged, and what is not. The agent service is enabled and started
# first, and the passphrase is put into the database with KeePassXC's own
# command-line tool, since the one thing a container cannot supply is a person
# typing at a console — which is how a passphrase gets there the first time.
# That first time is therefore not what this scenario judges. Everything after
# it is: a real database, a real keepassxc-cli, a real ssh-add and a real agent,
# and a key that had to come out of that database to get there, since with
# nobody to ask there is no other way in.
#
# Seeding through keepassxc-cli rather than through SSHakku is what makes it a
# check of the product rather than of itself: SSHakku's own writing is not
# involved in the seed, so the passphrase has to survive being written by one
# program and read by another.
#
# The database has no password on it and opens on its key file alone. That is
# not a shortcut around the prompt: it is the arrangement F24 describes, and the
# only one under which this route can load keys at a login where nobody is at a
# console. A database with a password would ask, and asking is a promise of its
# own that no container can answer — see the matrix.
#
# Every program below is run through Start-Process rather than the call
# operator, and its output read back from files. A native program's standard
# error reaching this shell directly is rendered as an error record and wrapped
# to the width of a console, which splits a one-line message into two and would
# have this scenario judging its own formatting rather than what was written.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

[CmdletBinding()]
param(
    [string] $Sshakku = 'C:\sshakku-under-test\sshakku.exe',
    [string] $ConfigTemplate = 'C:\scenario\windows-keepassxc-config.toml'
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$failures = [System.Collections.Generic.List[string]]::new()

$passphrase = 'correct-horse-battery-staple'
$keyName = 'id_keepassxc'
$service = "SSHakku-Key-$keyName"
$group = 'SSHakku'
$entry = "$group/$service"
$stranger = 'SomebodyElses-Entry'

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
        [string] $StdinText = '',
        [int] $TimeoutSeconds = 120
    )

    $inFile = [System.IO.Path]::GetTempFileName()
    $outFile = [System.IO.Path]::GetTempFileName()
    $errFile = [System.IO.Path]::GetTempFileName()
    try {
        # LF endings and no byte-order mark: this stands in for what a program
        # writes down a pipe, and keepassxc-cli reads a mark as part of the
        # password.
        [System.IO.File]::WriteAllText($inFile, $StdinText, (
                Microsoft.PowerShell.Utility\New-Object System.Text.UTF8Encoding $false))

        $p = Microsoft.PowerShell.Management\Start-Process -FilePath $FilePath `
            -ArgumentList $Arguments -NoNewWindow -PassThru `
            -RedirectStandardInput $inFile -RedirectStandardOutput $outFile -RedirectStandardError $errFile
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
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $inFile, $outFile, $errFile `
            -Force -ErrorAction SilentlyContinue
    }
}

# keepassxc-cli against the scenario's own database, opened the way SSHakku
# opens it: on the key file, with no password anywhere.
function Invoke-Kpxc {
    param(
        [string[]] $Arguments,
        [string] $StdinText = ''
    )

    Invoke-Native -FilePath 'keepassxc-cli' -Arguments (
        $Arguments + @('--no-password', '--key-file', $script:keyFile)) -StdinText $StdinText
}

# What the database holds, asked of KeePassXC rather than of SSHakku: a report
# that its own entry is gone is not the same as the entry being gone.
function Get-DatabaseListing {
    $listed = Invoke-Kpxc -Arguments @('ls', '-q', '-R', $script:database)
    "$($listed.Out)$($listed.Err)"
}

# A key of this account's own, locked with a passphrase — which is the point
# here: a key that opens without one proves nothing about a wallet.
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
        '-t', 'ed25519', '-N', $Passphrase, '-C', 'sshakku-keepassxc-scenario', '-f', $Path, '-q')
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

Microsoft.PowerShell.Utility\Write-Output '--- a machine with KeePassXC on it ---'

# KeePassXC's own installer records this entry; the image cannot, because it
# carries no step that runs a program (see windows-keepassxc.Dockerfile), so the
# machine is finished being set up here. Both the machine's stored value and
# this session's own are set: the first is what the claim "installed on this
# machine" means, the second is what the programs started below actually search.
$keepassxcDir = 'C:\KeePassXC'
$machinePath = [System.Environment]::GetEnvironmentVariable('PATH', [System.EnvironmentVariableTarget]::Machine)
if (($machinePath -split ';') -notcontains $keepassxcDir) {
    [System.Environment]::SetEnvironmentVariable(
        'PATH', "$machinePath;$keepassxcDir", [System.EnvironmentVariableTarget]::Machine)
}
$env:PATH = "$env:PATH;$keepassxcDir"

Microsoft.PowerShell.Utility\Write-Output '--- and an agent running on it ---'

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

Microsoft.PowerShell.Utility\Write-Output '--- a database whose only key is a key file ---'

# Asked before anything depends on it, and by bare name, which is how SSHakku
# runs it. A tool that is not on PATH, or that is there and cannot start, fails
# every step below at once, and every one of those failures would otherwise read
# as the product's.
#
# What it wrote is reported along with what it exited, because the two ways this
# fails look identical from here and are put right differently: a program that
# cannot resolve its imports dies before main with both streams empty, while one
# that ran and objected says something.
$tool = Invoke-Native -FilePath 'keepassxc-cli' -Arguments @('--version')
if ($tool.ExitCode -ne 0) {
    $failures.Add("precondition: keepassxc-cli exited $($tool.ExitCode) rather than reporting its version" +
        " — out [$("$($tool.Out)".Trim())] err [$("$($tool.Err)".Trim())]." +
        ' Both empty means it never started: the image is missing something it links against')
} else {
    Microsoft.PowerShell.Utility\Write-Output "keepassxc-cli: $("$($tool.Out)".Trim())"
}

$wallet = Microsoft.PowerShell.Management\Join-Path -Path $env:USERPROFILE -ChildPath 'kpxc'
Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $wallet -Force | Microsoft.PowerShell.Core\Out-Null
$database = Microsoft.PowerShell.Management\Join-Path -Path $wallet -ChildPath 'secrets.kdbx'
$keyFile = Microsoft.PowerShell.Management\Join-Path -Path $wallet -ChildPath 'secrets.keyx'

$created = Invoke-Native -FilePath 'keepassxc-cli' -Arguments @('db-create', '-q', '--set-key-file', $keyFile, $database)
if ($created.ExitCode -ne 0) {
    $failures.Add("precondition: no database could be made: $("$($created.Err)".Trim())")
}
Microsoft.PowerShell.Utility\Write-Output "database: $(Microsoft.PowerShell.Management\Test-Path -LiteralPath $database)"

$made = Invoke-Kpxc -Arguments @('mkdir', '-q', $database, $group)
if ($made.ExitCode -ne 0) {
    $failures.Add("precondition: the group SSHakku keeps its entries in could not be made: $("$($made.Err)".Trim())")
}

# The passphrase, put there by KeePassXC's own tool rather than by SSHakku.
$seeded = Invoke-Kpxc -Arguments @('add', '-q', '-p', $database, $entry) -StdinText "$passphrase`n"
if ($seeded.ExitCode -ne 0) {
    $failures.Add("precondition: the passphrase could not be seeded: $("$($seeded.Err)".Trim())")
}

# Somebody else's entry, outside the group SSHakku made for itself. F27 is about
# exactly this: forgetting everything forgets everything *of SSHakku's*.
$other = Invoke-Kpxc -Arguments @('add', '-q', '-p', $database, $stranger) -StdinText "not-ours-to-touch`n"
if ($other.ExitCode -ne 0) {
    $failures.Add("precondition: a second entry could not be added: $("$($other.Err)".Trim())")
}

Microsoft.PowerShell.Utility\Write-Output '--- the configuration that names that wallet ---'

# Where this account's configuration lives is the product's to say, not this
# scenario's to assemble from a rule it would then be testing against itself.
$configDir = ''
$configOut = (Invoke-Native -FilePath $Sshakku -Arguments @('config')).Out
if ("$configOut" -match 'config directory:\s*(?<dir>.+)') { $configDir = $Matches['dir'].Trim() }
if (-not $configDir) {
    Microsoft.PowerShell.Utility\Write-Output $configOut
    $failures.Add('sshakku config named no configuration directory, so there is nowhere to put one')
} else {
    Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $configDir -Force | Microsoft.PowerShell.Core\Out-Null
    $toml = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $ConfigTemplate -Raw).
    Replace('@DATABASE@', $database.Replace('\', '\\')).
    Replace('@KEY_FILE@', $keyFile.Replace('\', '\\'))
    [System.IO.File]::WriteAllText(
        (Microsoft.PowerShell.Management\Join-Path -Path $configDir -ChildPath 'config.toml'),
        $toml, (Microsoft.PowerShell.Utility\New-Object System.Text.UTF8Encoding $false))
    Microsoft.PowerShell.Utility\Write-Output $toml
}

$sshDir = Microsoft.PowerShell.Management\Join-Path -Path $env:USERPROFILE -ChildPath '.ssh'
Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $sshDir -Force | Microsoft.PowerShell.Core\Out-Null
$keyPath = Microsoft.PowerShell.Management\Join-Path -Path $sshDir -ChildPath $keyName
$keyFP = Initialize-Key -Path $keyPath -Passphrase $passphrase

$before = Get-AgentListing
if ($keyFP -and $before -match [System.Text.RegularExpressions.Regex]::Escape($keyFP)) {
    $failures.Add('precondition: the key was already in the agent, so its being there afterwards proves nothing')
}

Microsoft.PowerShell.Utility\Write-Output '--- the report, before anything is loaded ---'

$doctor = Invoke-Native -FilePath $Sshakku -Arguments @('doctor')
Microsoft.PowerShell.Utility\Write-Output $doctor.Out
# Matched where the report states the wallet in use, not merely anywhere the
# word appears: a report that named keepassxc only to say it will not be used
# would satisfy a looser check while describing the opposite outcome.
if ("$($doctor.Out)" -notmatch 'backend:\s+keepassxc\s+\(route: cli\)') {
    $failures.Add('doctor does not report keepassxc by the cli route as the wallet in use, so this platform refused the name')
}
if ("$($doctor.Out)" -notmatch 'keepassxc-cli:\s+found') {
    $failures.Add('doctor does not find the tool this route runs, which is what a user would have to install')
}
if ("$($doctor.Out)" -notmatch 'database:\s+found') {
    $failures.Add('doctor does not find the database this route opens, which the configuration had to name')
}

Microsoft.PowerShell.Utility\Write-Output '--- the key loads, with nobody asked anything ---'

# Where this account's session log lives is the product's to say.
$init = Invoke-Native -FilePath $Sshakku -Arguments @('shell-init', '--shell', 'powershell')
$log = ''
if ($init.Out -match "log_file\s*=\s*'(?<path>[^']+)'") { $log = $Matches['path'] }
if (-not $log) {
    Microsoft.PowerShell.Utility\Write-Output $init.Out
    $failures.Add('shell-init named no session log, so what the product wrote down cannot be read')
}

$load = Invoke-Native -FilePath $Sshakku -Arguments @('load-keys') -TimeoutSeconds 60
Microsoft.PowerShell.Utility\Write-Output "load-keys exited $($load.ExitCode)"
if ($load.TimedOut) {
    $failures.Add('load-keys never came back: with the passphrase in the database and no password on it there was nothing to ask anybody, so waiting means it asked')
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
    $failures.Add('a key whose passphrase was in the database did not reach the agent, and there was nobody to ask for it')
}

if ($log -and (Microsoft.PowerShell.Management\Test-Path -LiteralPath $log -PathType Leaf)) {
    $lines = Microsoft.PowerShell.Management\Get-Content -LiteralPath $log
    Microsoft.PowerShell.Utility\Write-Output ($lines -join [System.Environment]::NewLine)

    # Which way the key got in matters as much as its being in: a key added some
    # other way would look identical in the listing above.
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

$held = Get-DatabaseListing
Microsoft.PowerShell.Utility\Write-Output "the database now holds:`n$($held.Trim())"
if ($held -match [System.Text.RegularExpressions.Regex]::Escape($service)) {
    $failures.Add('the entry SSHakku stored is still in the database after it reported forgetting everything')
}
if ($held -notmatch [System.Text.RegularExpressions.Regex]::Escape($stranger)) {
    $failures.Add('an entry another program put in the database was taken out along with ours')
}

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: F22 holds here — KeePassXC named by name on Windows, a key loaded out of it with nothing typed, and forgetting took only what SSHakku put there'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
