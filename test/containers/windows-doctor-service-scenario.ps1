#!/usr/bin/env pwsh
#
# SSHakku's report met on a machine whose agent is a service that nothing may
# start — the state a Windows Server image ships in, and the state a machine is
# left in by an administrator who turned the agent off.
#
# Verifies F55, in the order a person meets it: the report first, which must
# name the service, say it is disabled, name what puts it right, and leave the
# service exactly as it found it; then `--fix`, which enables it and starts it.
# The two halves are one journey rather than two fixtures — the state the report
# described is the state --fix is then handed.
#
# The report leaving the machine alone is checked and not assumed, because it is
# the half that cannot be seen by reading the output: a diagnostic that repaired
# what it was asked to describe would print exactly the same words on the way.
#
# This runs as ContainerAdministrator, so the branch it drives is the one where
# the session has what enabling a service takes. The other branch — a session
# that has not — is measured on any developer's machine and on the CI runner by
# TestAnOrdinaryAccountMayNotOpenTheAgentsServiceToChangeIt, which asks for the
# handle and never writes. Between them the two answers are both covered, and
# neither machine can cover both.
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

function Get-ServiceStartMode {
    (CimCmdlets\Get-CimInstance -ClassName Win32_Service -Filter "Name='ssh-agent'").StartMode
}

function Get-ServiceStatus {
    (Microsoft.PowerShell.Management\Get-Service -Name ssh-agent).Status.ToString()
}

Microsoft.PowerShell.Utility\Write-Output '--- the machine this starts on ---'

# The image ships the service disabled, which is the state under test. Putting
# it there explicitly rather than trusting the image keeps the scenario honest
# about what it arranged, and lets it run twice in one container.
Microsoft.PowerShell.Management\Set-Service -Name ssh-agent -StartupType Disabled
$startMode = Get-ServiceStartMode
if ($startMode -ne 'Disabled') {
    $failures.Add("the scenario must start from a disabled service, and this one is $startMode")
}
Microsoft.PowerShell.Utility\Write-Output "ssh-agent: $(Get-ServiceStatus), start mode $startMode"

Microsoft.PowerShell.Utility\Write-Output '--- the report ---'

$report = Invoke-Native -FilePath $Sshakku -Arguments @('doctor')
Microsoft.PowerShell.Utility\Write-Output $report.Out

if ($report.ExitCode -ne 0) {
    $failures.Add("a report always arrives whatever it found, and this one exited $($report.ExitCode)")
}
if ($report.Out -notmatch 'agent service:') {
    $failures.Add('the report has no section for what serves the agent')
}
if ($report.Out -notmatch 'ssh-agent') {
    $failures.Add('the report does not name the service, which is the name an administrator acts on')
}
if ($report.Out -notmatch 'disabled') {
    $failures.Add('the report does not say the service is disabled, which asking its state alone never shows')
}
if ($report.Out -notmatch 'doctor --fix') {
    $failures.Add('the report does not name what puts it right')
}
if ($report.Out -notmatch 'administrator') {
    $failures.Add('the report does not say what running that takes')
}

# The old advice, and the defect this is here for: on this machine no login
# shell starts anything, so being sent to open one is being sent to do the one
# thing that cannot work.
if ($report.Out -match 'a new login shell starts one') {
    $failures.Add('the report sends the reader to open a shell that will start nothing')
}

# A list this system does not keep is not a list the report failed to read.
if ($report.Out -match 'report is partial') {
    $failures.Add('the report calls itself partial for a list this machine was never going to keep')
}
if ($report.Out -match 'ssh-agent processes \(0\)') {
    $failures.Add('the report counts nothing, which is a claim to have looked')
}

# The half that cannot be read off the output: F41's report is a look, not an
# action, and a diagnostic that repaired what it described would have printed
# exactly these words on the way.
$afterReport = Get-ServiceStartMode
if ($afterReport -ne 'Disabled') {
    $failures.Add("a report must leave the service as it found it, and this one left it $afterReport")
}

Microsoft.PowerShell.Utility\Write-Output '--- the repair ---'

$fix = Invoke-Native -FilePath $Sshakku -Arguments @('doctor', '--fix')
Microsoft.PowerShell.Utility\Write-Output $fix.Out

if ($fix.ExitCode -ne 0) {
    $failures.Add("a repair that succeeded exits as one, and this exited $($fix.ExitCode)")
}
if ($fix.Out -notmatch 'enabled the ssh-agent service') {
    $failures.Add('the run does not say which change it made to the machine')
}
$afterFix = Get-ServiceStartMode
if ($afterFix -ne 'Auto') {
    $failures.Add("the service should start by itself afterwards, and its start mode is $afterFix")
}
$statusAfterFix = Get-ServiceStatus
if ($statusAfterFix -ne 'Running') {
    $failures.Add("enabling without starting leaves the session no agent, and the service is $statusAfterFix")
}

# The report --fix prints once it has finished describes the machine it left
# behind, which is the only way a repair can be seen to have taken.
$after = ($fix.Out -split "`nafter:`n", 2)[-1]
if ($after -notmatch 'running, starts automatically') {
    $failures.Add('the report afterwards does not describe the service that was just repaired')
}
if ($after -match 'doctor --fix') {
    $failures.Add('the report afterwards still offers a repair that has been made')
}

Microsoft.PowerShell.Utility\Write-Output '--- the agent a session now gets ---'

# An agent holding no keys says so and exits 1, which is an answer. One that
# cannot be reached at all exits 2, which is what this is here to tell apart.
$agent = Invoke-Native -FilePath 'ssh-add.exe' -Arguments @('-l')
if ($agent.ExitCode -eq 2) {
    $failures.Add('ssh-add could not reach an agent at all after the repair')
}
Microsoft.PowerShell.Utility\Write-Output "ssh-add -l exited $($agent.ExitCode)"

Microsoft.PowerShell.Utility\Write-Output '--- result ---'

if ($failures.Count -gt 0) {
    Microsoft.PowerShell.Utility\Write-Output "$($failures.Count) failure(s)"
    exit 1
}

Microsoft.PowerShell.Utility\Write-Output 'PASS'
exit 0
