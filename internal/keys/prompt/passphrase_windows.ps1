# Asks for one key's passphrase in a window and writes what was typed to
# standard output, with nothing else on it.
#
# The exit code carries the rest of the answer, and only the two this script
# chooses mean anything: 0 is an answer, 2 is the box closed unanswered. Every
# other code belongs to the host — a policy that will not load this file, an
# edition with no WinForms to load — and the caller reads those as "could not
# ask" rather than as a dismissal. A machine where the window never draws must
# not look like somebody closing it, or one refusal would be taken for a
# decision and the asking would stop with nothing having appeared on screen.
#
# The key's name arrives as an argument rather than being written into this
# file. Key files are named by whoever put them in the key directory, and a name
# pasted into the source could close the string it landed in and carry on as
# PowerShell of its own.
#
# Cmdlets and types are named in full and the strict mode is set, as everywhere
# else this project runs PowerShell: this file runs in whichever host is
# installed, where a function or an alias of the same name is reached before the
# built-in one.

param(
    [Parameter(Mandatory = $true)][string]$KeyName
)

Microsoft.PowerShell.Core\Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# The passphrase leaves through standard output, so it is written as UTF-8
# whatever code page the machine is set to; without this, anything outside ASCII
# arrives as something else. A host that may not touch this type cannot load
# WinForms either, and failing here is the honest answer: the caller reads it as
# "could not ask" and finds somewhere else to ask, which is better than handing
# back a passphrase in an encoding nobody chose.
[System.Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Microsoft.PowerShell.Utility\Add-Type -AssemblyName System.Windows.Forms
Microsoft.PowerShell.Utility\Add-Type -AssemblyName System.Drawing

$form = [System.Windows.Forms.Form]::new()
$form.Text = 'SSHakku'
$form.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::FixedDialog
$form.StartPosition = [System.Windows.Forms.FormStartPosition]::CenterScreen
$form.MinimizeBox = $false
$form.MaximizeBox = $false
# The window is raised by a program the user did not start themselves, so it
# says so above whatever they are looking at rather than waiting behind it.
$form.TopMost = $true
$form.ClientSize = [System.Drawing.Size]::new(380, 130)

$label = [System.Windows.Forms.Label]::new()
$label.Text = "Enter passphrase for $KeyName"
$label.AutoSize = $true
$label.Location = [System.Drawing.Point]::new(12, 15)
$form.Controls.Add($label)

$box = [System.Windows.Forms.TextBox]::new()
$box.UseSystemPasswordChar = $true
$box.Location = [System.Drawing.Point]::new(15, 45)
$box.Size = [System.Drawing.Size]::new(350, 24)
$form.Controls.Add($box)

$ok = [System.Windows.Forms.Button]::new()
$ok.Text = 'OK'
$ok.DialogResult = [System.Windows.Forms.DialogResult]::OK
$ok.Location = [System.Drawing.Point]::new(200, 85)
$form.Controls.Add($ok)
$form.AcceptButton = $ok

$cancel = [System.Windows.Forms.Button]::new()
$cancel.Text = 'Cancel'
$cancel.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$cancel.Location = [System.Drawing.Point]::new(290, 85)
$form.Controls.Add($cancel)
$form.CancelButton = $cancel

# Nothing started this window from the keyboard, so it takes the focus itself
# and puts the caret where the typing is meant to go. Without this the first
# characters of a passphrase can land in whatever had the focus before.
$form.Add_Shown({
        $form.Activate()
        $box.Focus() | Microsoft.PowerShell.Core\Out-Null
    })

$result = $form.ShowDialog()

if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
    # Written rather than printed: the passphrase is handed over exactly as it
    # was typed, with no line ending added after it, since only its owner knows
    # whether it ends in a space.
    [System.Console]::Out.Write($box.Text)
    exit 0
}
exit 2
