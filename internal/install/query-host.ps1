# Reports what one PowerShell host knows about itself, as a single line of
# JSON on standard output: where its own profiles are, what execution policy
# governs it, and whether its language mode would let a dot-sourced hook run.
#
# It is run *inside* the host being asked, because every one of those answers
# belongs to that host and to no other. The two Windows editions disagree
# about the execution policy of the same account, from separate registry keys,
# so asking one and assuming the other is wrong; and no profile path can be
# assembled from a template, since a Documents folder redirected into
# OneDrive, a Store or portable installation, and two versions installed side
# by side each put the profiles somewhere only the host itself can say.
#
# The contract with the caller is narrow on purpose: one JSON object on
# standard output and nothing else. Anything this script cannot determine is
# left out of the object rather than reported as a guess.
#
# Every cmdlet below is named with its module and every type with its full
# namespace. This script runs inside a host it does not own, where a function,
# an alias or a `using namespace` of the same name is reached before the
# built-in one, and an answer about that host must not be shaped by it.

Microsoft.PowerShell.Core\Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

# Windows PowerShell writes progress records to standard error as a CLIXML
# payload, which is noise the caller has no use for.
$ProgressPreference = 'SilentlyContinue'

try {
    # And it renders standard output in the console's OEM code page unless
    # told otherwise, which mangles every non-ASCII character in a path.
    [System.Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
} catch {
    # A host in constrained language mode may not touch that type. The object
    # below still reports the mode, which is what tells the caller why a path
    # it reads back may not be the path on disk.
    Microsoft.PowerShell.Utility\Write-Debug "could not set the output encoding: $_"
}

# Values are interpolated rather than converted with a method call: an enum or
# a version renders the same either way, and interpolation is one of the few
# things a constrained host still allows.
$policies = [ordered]@{}
foreach ($entry in Microsoft.PowerShell.Security\Get-ExecutionPolicy -List) {
    $policies["$($entry.Scope)"] = "$($entry.ExecutionPolicy)"
}

[ordered]@{
    edition                  = "$($PSVersionTable.PSEdition)"
    version                  = "$($PSVersionTable.PSVersion)"
    psHome                   = "$PSHOME"
    languageMode             = "$($ExecutionContext.SessionState.LanguageMode)"
    effectiveExecutionPolicy = "$(Microsoft.PowerShell.Security\Get-ExecutionPolicy)"
    executionPolicyByScope   = $policies
    profiles                 = [ordered]@{
        default                = "$PROFILE"
        currentUserAllHosts    = "$($PROFILE.CurrentUserAllHosts)"
        currentUserCurrentHost = "$($PROFILE.CurrentUserCurrentHost)"
        allUsersAllHosts       = "$($PROFILE.AllUsersAllHosts)"
        allUsersCurrentHost    = "$($PROFILE.AllUsersCurrentHost)"
    }
} | Microsoft.PowerShell.Utility\ConvertTo-Json -Depth 5 -Compress
