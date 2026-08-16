#!/usr/bin/env pwsh
#
# Runs PSScriptAnalyzer over the PowerShell files named on the command line and
# exits non-zero if any of them produced a diagnostic — of any severity. This
# project has few enough PowerShell files that a warning left standing would
# become noise nobody reads, so there is no severity floor to argue about.
#
# The files come from the caller rather than being discovered here, so that
# one list of what is linted stays beside the other languages' lists.
#
# Every cmdlet below is named with its module and every type with its full
# namespace, which is the convention the files this checks are held to; a
# linter that did not follow the rule it enforces would be the wrong example
# to leave in the tree.

[CmdletBinding()]
param(
    [Parameter(Mandatory, ValueFromRemainingArguments)]
    [string[]] $Path
)

Microsoft.PowerShell.Core\Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Microsoft.PowerShell.Core\Import-Module -Name PSScriptAnalyzer

# One file at a time, so the file that failed is named by the line above its
# output rather than inferred from the paths inside it.
$failed = $false
foreach ($file in $Path) {
    Microsoft.PowerShell.Utility\Write-Output "Invoke-ScriptAnalyzer $file"
    $findings = PSScriptAnalyzer\Invoke-ScriptAnalyzer -Path $file
    if ($findings) {
        $findings |
            Microsoft.PowerShell.Utility\Format-Table -AutoSize -Property Severity, Line, RuleName, Message |
            Microsoft.PowerShell.Utility\Out-String -Width 200 |
            Microsoft.PowerShell.Utility\Write-Output
        $failed = $true
    }
}

if ($failed) {
    exit 1
}
