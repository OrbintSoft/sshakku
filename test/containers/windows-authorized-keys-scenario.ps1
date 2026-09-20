#!/usr/bin/env pwsh
#
# What an SSH server does with a key file it cannot read.
#
# This scenario does not run SSHakku. It holds the platform behaviour F69's
# shape rests on, which is a fact about `sshd` rather than about this program:
# encrypting a key directory to one account is safe for the private keys in it
# and is not safe for `authorized_keys`, because that file is read by `sshd`
# running as the system, before there is a session for it to borrow an identity
# from. Encrypt it and key logins to that account stop working — silently, since
# a file `sshd` cannot open is indistinguishable, from the outside, from an
# account that authorised nobody.
#
# It is measured rather than quoted, and the control is what makes it a
# measurement: the same file is made readable again with nothing else changed,
# and the same login is run again. A first half that ends in "refused" proves
# nothing unless the second half shows the same key let in.
#
# The unreadability is produced with an ACL rather than with EFS, for one
# reason: EFS does not work in a Windows container at all. `cipher` reports
# `[ERR]`, the attribute never changes, and a marked directory answers that new
# files in it will not be encrypted. What `sshd` is being held to here is what
# it does when the file cannot be opened, which both routes reach; that EFS is
# one such route is measured on a real machine instead, and noted in
# docs/TEST-MATRIX.md.
#
# Denying SYSTEM has to be explicit. Leaving it out of the list is not enough,
# because SYSTEM's token carries BUILTIN\Administrators, so a grant to the
# administrators lets it straight back in - which is exactly how a first draft
# of this scenario passed both halves and showed nothing.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention this project holds its PowerShell to.

[CmdletBinding()]
param(
    [string] $OpenSSH = 'C:\Windows\System32\OpenSSH',
    [int] $Port = 2222
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$failures = [System.Collections.Generic.List[string]]::new()

$account = 'keyprobe'
$password = 'Pr0be-Passw0rd-x'
$sshdConfigDir = 'C:\ProgramData\ssh'
$sshdConfig = Microsoft.PowerShell.Management\Join-Path -Path $sshdConfigDir -ChildPath 'probe_sshd_config'
$sshdLog = Microsoft.PowerShell.Management\Join-Path -Path $sshdConfigDir -ChildPath 'probe_sshd.log'
$hostKey = Microsoft.PowerShell.Management\Join-Path -Path $sshdConfigDir -ChildPath 'ssh_host_ed25519_key'
$clientKey = 'C:\keyprobe_key'
$taskName = 'sshd-under-test'

$sshd = Microsoft.PowerShell.Management\Join-Path -Path $OpenSSH -ChildPath 'sshd.exe'
$ssh = Microsoft.PowerShell.Management\Join-Path -Path $OpenSSH -ChildPath 'ssh.exe'
$keygen = Microsoft.PowerShell.Management\Join-Path -Path $OpenSSH -ChildPath 'ssh-keygen.exe'
foreach ($program in @($sshd, $ssh, $keygen)) {
    if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $program -PathType Leaf)) {
        Microsoft.PowerShell.Utility\Write-Output "FAIL: this image has no $program, so nothing here could be driven"
        exit 1
    }
}

# Replaces a file's whole access list, so that what it grants is what is written
# here and nothing inherited. Denying is an ACE of its own rather than an
# omission, for the reason in the header.
function Set-OnlyAccess {
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Mandatory)] [string] $Path,
        [Parameter(Mandatory)] [string] $Owner,
        [Parameter(Mandatory)] [bool] $ReadableBySystem
    )

    if (-not $PSCmdlet.ShouldProcess($Path, 'replace the access list')) { return }

    $acl = Microsoft.PowerShell.Security\Get-Acl -LiteralPath $Path
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($rule in @($acl.Access)) { $acl.RemoveAccessRule($rule) | Microsoft.PowerShell.Core\Out-Null }
    $acl.AddAccessRule((
            [System.Security.AccessControl.FileSystemAccessRule]::new($Owner, 'FullControl', 'Allow')))
    if ($ReadableBySystem) {
        $acl.AddAccessRule((
                [System.Security.AccessControl.FileSystemAccessRule]::new('NT AUTHORITY\SYSTEM', 'FullControl', 'Allow')))
    } else {
        $acl.AddAccessRule((
                [System.Security.AccessControl.FileSystemAccessRule]::new('NT AUTHORITY\SYSTEM', 'Read', 'Deny')))
    }
    $acl.SetOwner([System.Security.Principal.NTAccount]::new($Owner))
    Microsoft.PowerShell.Security\Set-Acl -LiteralPath $Path -AclObject $acl
}

# Runs a program and hands back what it wrote and what it exited with.
#
# It goes through Start-Process with its streams redirected to files rather than
# through the call operator, because a native program's standard error arriving
# in this shell is rendered as an error record — and `ssh` writes an ordinary
# notice there on a first connection, which would end the scenario as a failure
# before it had asserted anything.
function Invoke-Native {
    param(
        [string] $FilePath,
        [string[]] $Arguments,
        [int] $TimeoutSeconds = 60
    )

    $outFile = [System.IO.Path]::GetTempFileName()
    $errFile = [System.IO.Path]::GetTempFileName()
    try {
        $p = Microsoft.PowerShell.Management\Start-Process -FilePath $FilePath `
            -ArgumentList $Arguments -NoNewWindow -PassThru `
            -RedirectStandardOutput $outFile -RedirectStandardError $errFile
        # Reading the handle is what makes the exit code readable afterwards.
        $null = $p.Handle
        $timedOut = -not $p.WaitForExit($TimeoutSeconds * 1000)
        if ($timedOut) {
            $p.Kill()
            $null = $p.WaitForExit(5000)
        }
        [pscustomobject]@{
            Out      = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $outFile -Raw)
            Err      = (Microsoft.PowerShell.Management\Get-Content -LiteralPath $errFile -Raw)
            ExitCode = $(if ($timedOut) { -1 } else { $p.ExitCode })
            TimedOut = $timedOut
        }
    } finally {
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $outFile, $errFile -Force -ErrorAction SilentlyContinue
    }
}

# One login attempt against a freshly started server, and what the server said
# about it. The server is started and stopped around each attempt so that its
# log holds that attempt and no other.
function Invoke-LoginAttempt {
    param([Parameter(Mandatory)] [string] $Account)

    Microsoft.PowerShell.Management\Remove-Item -LiteralPath $sshdLog -Force -ErrorAction SilentlyContinue
    ScheduledTasks\Start-ScheduledTask -TaskName $taskName
    Microsoft.PowerShell.Utility\Start-Sleep -Seconds 5

    $listening = @(NetTCPIP\Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue).Count

    $attempt = Invoke-Native -FilePath $ssh -Arguments @(
        '-p', "$Port", '-i', $clientKey,
        '-o', 'IdentitiesOnly=yes', '-o', 'IdentityAgent=none', '-o', 'StrictHostKeyChecking=no',
        '-o', 'UserKnownHostsFile=C:\known_hosts', '-o', 'PasswordAuthentication=no',
        '-o', 'BatchMode=yes', '-o', 'PreferredAuthentications=publickey',
        "$Account@127.0.0.1", 'whoami')
    $exit = $attempt.ExitCode

    ScheduledTasks\Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Microsoft.PowerShell.Management\Get-Process -Name sshd -ErrorAction SilentlyContinue |
        Microsoft.PowerShell.Management\Stop-Process -Force -ErrorAction SilentlyContinue
    Microsoft.PowerShell.Utility\Start-Sleep -Seconds 2

    $log = @(Microsoft.PowerShell.Management\Get-Content -LiteralPath $sshdLog -ErrorAction SilentlyContinue)
    return [pscustomobject]@{
        Listening = $listening
        ExitCode  = $exit
        Output    = "$($attempt.Out)"
        Notices   = "$($attempt.Err)"
        Log       = $log -join [System.Environment]::NewLine
    }
}

Microsoft.PowerShell.Utility\Write-Output '--- a server, an account, and a key of that account''s ---'

Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $sshdConfigDir -Force | Microsoft.PowerShell.Core\Out-Null
@(
    "Port $Port"
    'ListenAddress 127.0.0.1'
    "HostKey $hostKey"
    'PubkeyAuthentication yes'
    'PasswordAuthentication no'
    'LogLevel DEBUG2'
) | Microsoft.PowerShell.Management\Set-Content -LiteralPath $sshdConfig

& $keygen -A 2>&1 | Microsoft.PowerShell.Core\Out-Null
# OpenSSH refuses a host key it considers anyone else's to read.
Set-OnlyAccess -Path $hostKey -Owner 'NT AUTHORITY\SYSTEM' -ReadableBySystem $true

$secret = Microsoft.PowerShell.Security\ConvertTo-SecureString -String $password -AsPlainText -Force
Microsoft.PowerShell.LocalAccounts\New-LocalUser -Name $account -Password $secret `
    -AccountNeverExpires -PasswordNeverExpires -ErrorAction SilentlyContinue | Microsoft.PowerShell.Core\Out-Null
$credential = [System.Management.Automation.PSCredential]::new($account, $secret)
# A profile is what gives the account a home directory for sshd to look in, and
# logging on once is what creates one.
Microsoft.PowerShell.Management\Start-Process -FilePath 'cmd.exe' -ArgumentList '/c', 'exit' `
    -Credential $credential -Wait -ErrorAction SilentlyContinue
$accountHome = Microsoft.PowerShell.Management\Join-Path -Path 'C:\Users' -ChildPath $account
if (-not (Microsoft.PowerShell.Management\Test-Path -LiteralPath $accountHome)) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: the account never got a profile, so sshd would have nowhere to read from"
    exit 1
}

& $keygen -t ed25519 -f $clientKey -N '""' -q 2>&1 | Microsoft.PowerShell.Core\Out-Null
$keyDir = Microsoft.PowerShell.Management\Join-Path -Path $accountHome -ChildPath '.ssh'
Microsoft.PowerShell.Management\New-Item -ItemType Directory -Path $keyDir -Force | Microsoft.PowerShell.Core\Out-Null
$authorizedKeys = Microsoft.PowerShell.Management\Join-Path -Path $keyDir -ChildPath 'authorized_keys'
Microsoft.PowerShell.Management\Copy-Item -LiteralPath "$clientKey.pub" -Destination $authorizedKeys -Force

# sshd runs as the system here exactly as the service does, which is the whole
# point: an ordinary account's sshd could read its own files and would show
# nothing.
$action = ScheduledTasks\New-ScheduledTaskAction -Execute $sshd -Argument "-f $sshdConfig -D -E $sshdLog"
$principal = ScheduledTasks\New-ScheduledTaskPrincipal -UserId 'NT AUTHORITY\SYSTEM' `
    -LogonType ServiceAccount -RunLevel Highest
ScheduledTasks\Register-ScheduledTask -TaskName $taskName -Action $action -Principal $principal -Force |
    Microsoft.PowerShell.Core\Out-Null

Microsoft.PowerShell.Utility\Write-Output '--- with a key file the server cannot read ---'
Set-OnlyAccess -Path $authorizedKeys -Owner $account -ReadableBySystem $false
$refused = Invoke-LoginAttempt -Account $account
Microsoft.PowerShell.Utility\Write-Output "ssh exited $($refused.ExitCode)"

if ($refused.Listening -ne 1) {
    $failures.Add('no server was listening for the first attempt, so its refusal says nothing')
}
if ($refused.ExitCode -eq 0) {
    $failures.Add('the login went through against a key file the server cannot read, which is the whole premise of F69 refusing to mark that directory')
}
if ($refused.Log -notmatch 'Could not open authorized keys') {
    Microsoft.PowerShell.Utility\Write-Output $refused.Log
    $failures.Add('the server did not report being unable to open authorized_keys, so the refusal above may have another cause')
}

Microsoft.PowerShell.Utility\Write-Output '--- with the same file readable, and nothing else changed ---'
Set-OnlyAccess -Path $authorizedKeys -Owner $account -ReadableBySystem $true
$allowed = Invoke-LoginAttempt -Account $account
Microsoft.PowerShell.Utility\Write-Output "ssh exited $($allowed.ExitCode)"

if ($allowed.ExitCode -ne 0) {
    Microsoft.PowerShell.Utility\Write-Output $allowed.Notices
    Microsoft.PowerShell.Utility\Write-Output $allowed.Log
    $failures.Add('the same key was refused even with the file readable, so the first half is not attributable to the file')
}
if ($allowed.Log -notmatch 'Accepted key') {
    $failures.Add('the server never reported accepting the key, so nothing above is about reading authorized_keys')
}
if ($allowed.Output -notmatch $account) {
    $failures.Add("the session did not come back as $account, so no login actually happened")
}

ScheduledTasks\Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue

Microsoft.PowerShell.Utility\Write-Output '--- result ---'
if ($failures.Count -eq 0) {
    Microsoft.PowerShell.Utility\Write-Output 'PASS: a key file the SSH server cannot read is a key login refused, and the same file readable is the same login let in'
    exit 0
}

foreach ($f in $failures) {
    Microsoft.PowerShell.Utility\Write-Output "FAIL: $f"
}
exit 1
