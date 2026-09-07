# Opens one PowerShell session in the shape a test asks for, has it run the
# script named by -Runner, and waits for it to end.
#
# Test scaffolding. It arranges how a session is started and nothing about what
# that session then decides: a session that had to be told it was somebody's
# would prove nothing about how the product tells them apart.
#
# The shape is given in two parts because they are two different things the
# product reads. What the session was told on its command line — whether it was
# handed work, and whether it was told to stay afterwards — and where it reads
# from. A console is what a session gets when nothing captures its streams:
# Start-Process hands a child the standard handles this script was itself given
# the moment any of them is captured, and those are pipes, which is exactly what
# the product reads to conclude that nobody is sitting there.
param(
    [Parameter(Mandatory = $true)][string]$Interpreter,
    [Parameter(Mandatory = $true)][string]$Runner,
    [Parameter(Mandatory = $true)][ValidateSet('command', 'file')][string]$Handed,
    [switch]$Stays,
    [switch]$MayNotAsk,
    [switch]$AtAConsole
)

$sessionArgs = @('-NoProfile')
if ($Stays) {
    $sessionArgs += '-noexit'
}
if ($MayNotAsk) {
    $sessionArgs += '-NonInteractive'
}
if ($Handed -eq 'command') {
    # The call operator, because -Command reads what follows as a command and a
    # quoted path on its own is a string expression: the session would print the
    # path and run nothing. This is the shape a terminal uses to run its own
    # startup script before handing the window over.
    $sessionArgs += @('-command', "& '" + $Runner.Replace("'", "''") + "'")
} else {
    # Quoted here rather than left to Start-Process, which joins the argument
    # list with spaces and quotes nothing, so a path holding one would arrive as
    # two arguments.
    $sessionArgs += @('-File', '"' + $Runner + '"')
}

if ($AtAConsole) {
    $session = Microsoft.PowerShell.Management\Start-Process -FilePath $Interpreter `
        -ArgumentList $sessionArgs -WindowStyle Hidden -PassThru
    # Bounded, because a session that reaches its prompt is one nothing here can
    # type into: it would hold the whole run open rather than fail it.
    if (-not $session.WaitForExit(120000)) {
        $session.Kill()
        throw 'the session opened at a console never ended'
    }
    exit $session.ExitCode
}

& $Interpreter @sessionArgs
exit $LASTEXITCODE
