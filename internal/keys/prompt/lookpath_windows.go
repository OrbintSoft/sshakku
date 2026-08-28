//go:build windows

package prompt

import "os/exec"

// execLookPath resolves a binary on PATH; it is a variable so tests can stub the
// PATH lookup. It lives beside the dialogs because they are what asks: a
// prompter that runs a program has to find out whether that program is here,
// and this platform has two hosts either of which could be the one installed.
var execLookPath = exec.LookPath
